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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/gitiles"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

// gitilesRoot is the root for a FUSE filesystem backed by a Gitiles
// service.
type gitilesRoot struct {
	nodefs.Node

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

type linkNode struct {
	nodefs.Node
	linkTarget []byte
}

func (n *linkNode) Deletable() bool { return false }

func newLinkNode(target string) *linkNode {
	return &linkNode{
		Node:       nodefs.NewDefaultNode(),
		linkTarget: []byte(target),
	}
}

func (n *linkNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) (code fuse.Status) {
	out.Size = uint64(len(n.linkTarget))
	out.Mode = fuse.S_IFLNK

	t := time.Unix(1, 0)
	out.SetTimes(nil, &t, nil)

	return fuse.OK
}

func (n *linkNode) Readlink(c *fuse.Context) ([]byte, fuse.Status) {
	return n.linkTarget, fuse.OK
}

// gitilesNode represents a read-only blob in the FUSE filesystem.
type gitilesNode struct {
	nodefs.Node

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

func (n *gitilesNode) Deletable() bool {
	return false
}

func (n *gitilesNode) Utimens(file nodefs.File, atime *time.Time, mtime *time.Time, context *fuse.Context) (code fuse.Status) {
	if mtime != nil {
		n.mtimeMu.Lock()
		n.mtime = *mtime
		n.mtimeMu.Unlock()
	}
	return fuse.OK
}

func (n *gitilesNode) Readlink(c *fuse.Context) ([]byte, fuse.Status) {
	return n.linkTarget, fuse.OK
}

func (n *gitilesNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) (code fuse.Status) {
	out.Size = uint64(n.size)
	out.Mode = n.mode

	n.mtimeMu.Lock()
	t := n.mtime
	n.mtimeMu.Unlock()

	out.SetTimes(nil, &t, nil)
	return fuse.OK
}

const xattrName = "user.gitsha1"

func (n *gitilesNode) GetXAttr(attribute string, context *fuse.Context) (data []byte, code fuse.Status) {
	if attribute != xattrName {
		return nil, fuse.ENODATA
	}
	return []byte(n.id.String()), fuse.OK
}

func (n *gitilesNode) ListXAttr(context *fuse.Context) (attrs []string, code fuse.Status) {
	return []string{xattrName}, fuse.OK
}

func (n *gitilesNode) Open(flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	if n.root.handleLessIO {
		// We say ENOSYS so FUSE on Linux uses handle-less I/O.
		return nil, fuse.ENOSYS
	}

	f, err := n.root.openFile(n.id, n.clone)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}

	return &nodefs.WithFlags{
		File:      nodefs.NewLoopbackFile(f),
		FuseFlags: fuse.FOPEN_KEEP_CACHE,
	}, fuse.OK
}

func (n *gitilesNode) Read(file nodefs.File, dest []byte, off int64, context *fuse.Context) (fuse.ReadResult, fuse.Status) {
	if off == 0 {
		atomic.AddUint32(&n.readCount, 1)
	}

	if n.root.handleLessIO {
		return n.handleLessRead(file, dest, off, context)
	}

	return file.Read(dest, off)
}

func (n *gitilesNode) handleLessRead(file nodefs.File, dest []byte, off int64, context *fuse.Context) (fuse.ReadResult, fuse.Status) {
	// TODO(hanwen): for large files this is not efficient. Should
	// have a cache of open file handles.
	f, err := n.root.openFile(n.id, n.clone)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}

	m, err := f.ReadAt(dest, off)
	if err == io.EOF {
		err = nil
	}
	f.Close()
	return fuse.ReadResultData(dest[:m]), fuse.ToStatus(err)
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
	nodefs.Node
	data []byte
}

func (n *dataNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) (code fuse.Status) {
	out.Size = uint64(len(n.data))
	out.Mode = fuse.S_IFREG | 0644
	t := time.Unix(1, 0)
	out.SetTimes(nil, &t, nil)

	return fuse.OK
}

func (n *dataNode) Open(flags uint32, content *fuse.Context) (nodefs.File, fuse.Status) {
	return nodefs.NewDataFile(n.data), fuse.OK
}

func (n *dataNode) GetXAttr(attribute string, context *fuse.Context) (data []byte, code fuse.Status) {
	return nil, fuse.ENODATA
}

func (n *dataNode) Deletable() bool { return false }

func newDataNode(c []byte) nodefs.Node {
	return &dataNode{nodefs.NewDefaultNode(), c}
}

// NewGitilesRoot returns the root node for a file system.
func NewGitilesRoot(c *cache.Cache, tree *gitiles.Tree, service *gitiles.RepoService, options GitilesRevisionOptions) nodefs.Node {
	r := &gitilesRoot{
		Node:         newDirNode(),
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

func (r *gitilesRoot) Deletable() bool { return false }

func (r *gitilesRoot) GetXAttr(attribute string, context *fuse.Context) (data []byte, code fuse.Status) {
	return nil, fuse.ENODATA
}

func (r *gitilesRoot) OnMount(fsConn *nodefs.FileSystemConnector) {
	if err := r.onMount(fsConn); err != nil {
		log.Printf("onMount: %v", err)
		for k := range r.Inode().Children() {
			r.Inode().RmChild(k)
		}
		r.Inode().NewChild("ERROR", false, newDataNode([]byte(err.Error())))
	}
}

type dirNode struct {
	nodefs.Node
}

// Implement Utimens so we don't create spurious "not implemented"
// messages when directory targets for symlinks are touched.
func (n *dirNode) Utimens(file nodefs.File, atime *time.Time, mtime *time.Time, context *fuse.Context) (code fuse.Status) {
	return fuse.OK
}

func (n *dirNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) (code fuse.Status) {
	out.Mode = fuse.S_IFDIR | 0755
	t := time.Unix(1, 0)
	out.SetTimes(nil, &t, nil)
	return fuse.OK
}

func (n *dirNode) Deletable() bool {
	return false
}

func newDirNode() nodefs.Node {
	return &dirNode{nodefs.NewDefaultNode()}
}

func (r *gitilesRoot) pathTo(fsConn *nodefs.FileSystemConnector, dir string) *nodefs.Inode {
	parent, left := fsConn.Node(r.Inode(), dir)
	for _, l := range left {
		ch := parent.NewChild(l, true, newDirNode())
		parent = ch
	}
	return parent
}

func (r *gitilesRoot) onMount(fsConn *nodefs.FileSystemConnector) error {
	for _, e := range r.tree.Entries {
		if e.Type == "commit" {
			// TODO(hanwen): support submodules.  For now,
			// we pretend we are plain git, which also
			// leaves an empty directory in the place of a submodule.
			r.pathTo(fsConn, e.Name)
			continue
		}
		if e.Type != "blob" {
			log.Panicf("unexpected object type %s", e.Type)
		}

		p := e.Name
		dir, base := filepath.Split(p)

		parent := r.pathTo(fsConn, dir)
		id, err := parseID(e.ID)
		if err != nil {
			return err
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
				Node:  nodefs.NewDefaultNode(),
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

			if e.Target != nil {
				n.linkTarget = []byte(*e.Target)
				n.size = int64(len(n.linkTarget))
			}

			r.shaMap[*id] = p
			parent.NewChild(base, false, n)
			r.nodeCache.add(n)
		} else {
			parent.AddChild(base, n.Inode())
		}

	}

	slothfsNode := r.Inode().NewChild(".slothfs", true, newDirNode())
	slothfsNode.NewChild("treeID", false, newDataNode([]byte(r.tree.ID)))

	treeContent, err := json.MarshalIndent(r.tree, "", " ")
	if err != nil {
		log.Panicf("json.Marshal: %v", err)
	}

	slothfsNode.NewChild("tree.json", false, newDataNode([]byte(treeContent)))

	// We don't need the tree data anymore.
	r.tree = nil

	if fsConn.Server().KernelSettings().Flags&fuse.CAP_NO_OPEN_SUPPORT != 0 {
		r.handleLessIO = true
	}
	return nil
}
