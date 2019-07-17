// Copyright 2016 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/gitiles"
	"github.com/hanwen/go-fuse/fs"
	"github.com/hanwen/go-fuse/fuse"
)

// gitilesRoot is the root for a FUSE filesystem backed by a Gitiles
// service.
type gitilesRoot struct {
	fs.Inode

	nodeCache *nodeCache

	cache   *cache.Cache
	service *gitiles.RepoService
	tree    *gitiles.Tree
	opts    GitilesRevisionOptions

	handleLessIO bool

	// OID => path
	shaMap map[plumbing.Hash]string

	lazyRepo *cache.LazyRepo

	fetchingCond *sync.Cond
	fetching     map[plumbing.Hash]bool
}

// gitilesNode represents a read-only blob in the FUSE filesystem.
type gitilesNode struct {
	fs.Inode

	root *gitilesRoot

	// Data from Git metadata.
	mode       uint32
	size       int64
	id         plumbing.Hash
	linkTarget []byte

	// if set, clone the repo on reading this file.
	clone bool

	// The timestamp is writable; protect it with a mutex.
	mtimeMu sync.Mutex
	mtime   time.Time

	// This is to verify that FOPEN_KEEP_CACHE is working as expected.
	readCount uint32
}

var _ = (fs.NodeReadlinker)((*gitilesNode)(nil))

func (n *gitilesNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	return n.linkTarget, 0
}

var _ = (fs.NodeGetattrer)((*gitilesNode)(nil))

func (n *gitilesNode) Getattr(ctx context.Context, h fs.FileHandle, out *fuse.AttrOut) (code syscall.Errno) {
	out.Size = uint64(n.size)
	out.Mode = n.mode

	n.mtimeMu.Lock()
	t := n.mtime
	n.mtimeMu.Unlock()

	out.SetTimes(nil, &t, nil)
	return 0
}

var _ = (fs.NodeSetattrer)((*gitilesNode)(nil))

func (n *gitilesNode) Setattr(ctx context.Context, h fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) (code syscall.Errno) {
	if 0 != in.Valid&(fuse.FATTR_MODE|
		fuse.FATTR_UID|
		fuse.FATTR_GID|
		fuse.FATTR_SIZE|
		fuse.FATTR_LOCKOWNER|
		fuse.FATTR_CTIME) {
		return syscall.ENOTSUP
	}
	if mt, ok := in.GetMTime(); ok {
		n.mtimeMu.Lock()
		n.mtime = mt
		n.mtimeMu.Unlock()

		return n.Getattr(ctx, h, out)
	}
	return 0
}

const xattrName = "user.gitsha1"

var _ = (fs.NodeGetxattrer)((*gitilesNode)(nil))

func (n *gitilesNode) Getxattr(ctx context.Context, attribute string, dest []byte) (uint32, syscall.Errno) {
	if attribute != xattrName {
		return 0, syscall.ENODATA
	}
	sz := copy(dest, n.id.String())
	return uint32(sz), 0
}

var _ = (fs.NodeListxattrer)((*gitilesNode)(nil))

func (n *gitilesNode) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	sz := copy(dest, xattrName)
	dest[sz] = 0
	return uint32(sz + 1), 0
}

var _ = (fs.NodeOpener)((*gitilesNode)(nil))

func (n *gitilesNode) Open(ctx context.Context, flags uint32) (h fs.FileHandle, fuseFlags uint32, code syscall.Errno) {
	if n.root.handleLessIO {
		// We say ENOSYS so FUSE on Linux uses handle-less I/O.
		return nil, 0, syscall.ENOSYS
	}

	f, err := n.root.openFile(n.id, n.clone)
	if err != nil {
		return nil, 0, fs.ToErrno(err)
	}

	return fs.NewLoopbackFile(int(f.Fd())), fuse.FOPEN_KEEP_CACHE, 0
}

var _ = (fs.NodeReader)((*gitilesNode)(nil))

func (n *gitilesNode) Read(ctx context.Context, file fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if off == 0 {
		atomic.AddUint32(&n.readCount, 1)
	}

	if n.root.handleLessIO {
		return n.handleLessRead(file, dest, off)
	}

	return file.(fs.FileReader).Read(ctx, dest, off)
}

func (n *gitilesNode) handleLessRead(file fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	// TODO(hanwen): for large files this is not efficient. Should
	// have a cache of open file handles.
	f, err := n.root.openFile(n.id, n.clone)
	if err != nil {
		return nil, fs.ToErrno(err)
	}

	m, err := f.ReadAt(dest, off)
	if err == io.EOF {
		err = nil
	}
	f.Close()
	return fuse.ReadResultData(dest[:m]), fs.ToErrno(err)
}

// openFile returns a file handle for the given blob. If `clone` is
// given, we may try a clone of the git repository
func (r *gitilesRoot) openFile(id plumbing.Hash, clone bool) (*os.File, error) {
	f, ok := r.cache.Blob.Open(id)
	if ok {
		return f, nil
	}

	f, err := r.fetchFile(id, clone)
	if err != nil {
		log.Printf("fetchFile(%s): %v", id.String(), err)
		return nil, syscall.ESPIPE
	}

	return f, nil
}

func (r *gitilesRoot) fetchFile(id plumbing.Hash, clone bool) (*os.File, error) {
	r.fetchingCond.L.Lock()
	defer r.fetchingCond.L.Unlock()

	for r.fetching[id] {
		r.fetchingCond.Wait()
	}

	f, ok := r.cache.Blob.Open(id)
	if ok {
		return f, nil
	}

	r.fetching[id] = true
	defer func() { delete(r.fetching, id) }()
	r.fetchingCond.L.Unlock()
	err := r.fetchFileExpensive(id, clone)
	r.fetchingCond.L.Lock()
	r.fetchingCond.Broadcast()

	if err == nil {
		f, ok = r.cache.Blob.Open(id)
		if !ok {
			return nil, fmt.Errorf("fetch succeeded, but blob %s not there", id.String())
		}
		return f, nil
	}

	return nil, err
}

func readBlob(blob *object.Blob) ([]byte, error) {
	r, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return ioutil.ReadAll(r)
}

func (r *gitilesRoot) fetchFileExpensive(id plumbing.Hash, clone bool) error {
	repo := r.lazyRepo.Repository()
	if clone && repo == nil {
		r.lazyRepo.Clone()
	}

	var content []byte
	if repo != nil {
		blob, err := repo.BlobObject(id)
		if err == nil {
			content, err = readBlob(blob)
			if err != nil {
				content = nil
			}
		}
	}

	if content == nil {
		path := r.shaMap[id]

		var err error
		content, err = r.service.GetBlob(r.opts.Revision, path)
		if err != nil {
			return fmt.Errorf("GetBlob(%s, %s): %v", r.opts.Revision, path, err)
		}
	}

	if err := r.cache.Blob.Write(id, content); err != nil {
		return err
	}
	return nil
}

// dataNode makes arbitrary data available as a file.
type dataNode struct {
	fs.Inode
	data []byte
}

var _ = (fs.NodeGetattrer)((*dataNode)(nil))

func (n *dataNode) Getattr(ctx context.Context, file fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = uint64(len(n.data))
	out.Mode = fuse.S_IFREG | 0644
	t := time.Unix(1, 0)
	out.SetTimes(nil, &t, nil)

	return 0
}

var _ = (fs.NodeOpener)((*gitilesNode)(nil))

func (n *dataNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, syscall.Errno) {
	return fs.MemRegularFile{Data: n.data}, 0
}

var _ = (fs.NodeGetxattrer)((*gitilesNode)(nil))

func (n *dataNode) GetXAttr(ctx context.Context, attribute string) (data []byte, code syscall.Errno) {
	return nil, syscall.ENODATA
}

// NewGitilesRoot returns the root node for a file system.
func NewGitilesRoot(c *cache.Cache, tree *gitiles.Tree, service *gitiles.RepoService, options GitilesRevisionOptions) *gitilesRoot {
	r := &gitilesRoot{
		service:      service,
		nodeCache:    newNodeCache(),
		cache:        c,
		shaMap:       map[plumbing.Hash]string{},
		tree:         tree,
		opts:         options,
		lazyRepo:     cache.NewLazyRepo(options.CloneURL, c),
		fetchingCond: sync.NewCond(&sync.Mutex{}),
		fetching:     map[plumbing.Hash]bool{},
	}

	return r
}

var _ = (fs.NodeGetxattrer)((*gitilesRoot)(nil))

func (r *gitilesRoot) Getxattr(ctx context.Context, attribute string, data []byte) (sz uint32, code syscall.Errno) {
	return 0, syscall.ENODATA
}

func (r *gitilesRoot) pathTo(dir string) *fs.Inode {
	p := &r.Inode
	for _, c := range strings.Split(dir, "/") {
		if len(c) == 0 {
			continue
		}
		ch := p.GetChild(c)
		if ch == nil {
			ch = p.NewPersistentInode(context.Background(),
				&fs.Inode{},
				fs.StableAttr{Mode: syscall.S_IFDIR})
			p.AddChild(c, ch, true)
		}
		p = ch
	}
	return p
}

var _ = (fs.NodeOnAdder)((*gitilesRoot)(nil))

func (r *gitilesRoot) OnAdd(ctx context.Context) {
	for _, e := range r.tree.Entries {
		if e.Type == "commit" {
			// TODO(hanwen): support submodules.  For now,
			// we pretend we are plain git, which also
			// leaves an empty directory in the place of a submodule.
			r.pathTo(e.Name)
			continue
		}
		if e.Type != "blob" {
			log.Panicf("unexpected object type %s", e.Type)
		}

		p := e.Name
		dir, base := filepath.Split(p)

		parent := r.pathTo(dir)
		id, err := parseID(e.ID)
		if err != nil {
			return
		}

		// Determine if file should trigger a clone.
		clone := r.opts.CloneURL != ""
		if clone {
			for _, e := range r.opts.CloneOption {
				if e.RE.MatchString(p) {
					clone = e.Clone
					break
				}
			}
		}

		xbit := e.Mode&0111 != 0
		n := r.nodeCache.get(id, xbit)
		if n == nil {
			n = &gitilesNode{
				id:    *id,
				mode:  uint32(e.Mode),
				clone: clone,
				root:  r,
				// Ninja uses mtime == 0 as "doesn't exist"
				// flag, (see ninja/files/src/graph.h:66), so
				// use a nonzero timestamp here.
				mtime: time.Unix(1, 0),
			}
			if e.Size != nil {
				n.size = int64(*e.Size)
			}

			mode := uint32(syscall.S_IFREG)
			if e.Target != nil {
				n.linkTarget = []byte(*e.Target)
				n.size = int64(len(n.linkTarget))
				mode = syscall.S_IFLNK
			}

			r.shaMap[*id] = p

			ch := parent.NewPersistentInode(ctx, n, fs.StableAttr{Mode: mode})
			parent.AddChild(base, ch, true)
			r.nodeCache.add(n)
		} else {
			parent.AddChild(base, n.EmbeddedInode(), true)
		}

	}

	slothfsNode := r.NewPersistentInode(ctx, &fs.Inode{}, fs.StableAttr{Mode: syscall.S_IFDIR})
	r.AddChild(".slothfs", slothfsNode, true)
	idFile := r.NewPersistentInode(ctx, &fs.MemRegularFile{
		Data: []byte(r.tree.ID)}, fs.StableAttr{Mode: syscall.S_IFREG})

	slothfsNode.AddChild("treeID", idFile, false)

	treeContent, err := json.MarshalIndent(r.tree, "", " ")
	if err != nil {
		log.Panicf("json.Marshal: %v", err)
	}
	jsonFile := r.NewPersistentInode(ctx, &fs.MemRegularFile{
		Data: treeContent}, fs.StableAttr{Mode: syscall.S_IFREG})

	slothfsNode.AddChild("tree.json", jsonFile, false)

	// We don't need the tree data anymore.
	r.tree = nil

	// XXX
	//	if fsConn.Server().KernelSettings().Flags&fuse.CAP_NO_OPEN_SUPPORT != 0 {
	//		r.handleLessIO = true
	//	}
}
