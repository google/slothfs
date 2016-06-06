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
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/google/gitfs/cache"
	"github.com/google/gitfs/gitiles"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	git "github.com/libgit2/git2go"
)

// gitilesRoot is the root for a FUSE filesystem backed by a Gitiles
// service.
type gitilesRoot struct {
	nodefs.Node

	cache   *cache.Cache
	service *gitiles.RepoService
	tree    *gitiles.Tree
	opts    GitilesOptions

	// TODO(hanwen): enable this again. After mount, set this to
	// server.KernelSettings().Flags&fuse.CAP_NO_OPEN_SUPPORT != 0.
	// This requires a suitably new kernel, though.
	handleLessIO bool

	// OID => path
	shaMap map[git.Oid]string

	lazyRepo *cache.LazyRepo
}

// gitilesNode represents a read-only blob in the FUSE filesystem.
type gitilesNode struct {
	nodefs.Node

	root *gitilesRoot

	// Data from Git metadata.
	mode       uint32
	size       int64
	id         git.Oid
	linkTarget []byte

	// if set, clone the repo on reading this file.
	clone bool

	// The timestamp is writable; protect it with a mutex.
	mtimeMu sync.Mutex
	mtime   time.Time
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

// TODO(hanwen): implement extended attributes to read the SHA1.

func (n *gitilesNode) Open(flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	if n.root.handleLessIO {
		// We say ENOSYS so FUSE on Linux uses handle-less I/O.
		return nil, fuse.ENOSYS
	}

	f, err := n.root.openFile(n.id, n.clone)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	return nodefs.NewLoopbackFile(f), fuse.OK
}

func (n *gitilesNode) Read(file nodefs.File, dest []byte, off int64, context *fuse.Context) (fuse.ReadResult, fuse.Status) {
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
func (r *gitilesRoot) openFile(id git.Oid, clone bool) (*os.File, error) {
	f, ok := r.cache.Blob.Open(id)
	if !ok {
		repo := r.lazyRepo.Repository()
		if clone && repo == nil {
			r.lazyRepo.Clone()
		}

		var content []byte
		if repo != nil {
			blob, err := repo.LookupBlob(&id)
			if err != nil {
				log.Println("LookupBlob: %v", err)
				return nil, syscall.ESPIPE
			}
			content = blob.Contents()
		} else {
			path := r.shaMap[id]

			var err error
			content, err = r.service.GetBlob(r.opts.Revision, path)
			if err != nil {
				log.Printf("GetBlob(%s, %s): %v", r.opts.Revision, path, err)
				return nil, syscall.EDOM
			}
		}

		if err := r.cache.Blob.Write(id, content); err != nil {
			return nil, err
		}

		f, ok = r.cache.Blob.Open(id)
		if !ok {
			return nil, syscall.EROFS
		}
	}
	return f, nil
}

// dataNode makes arbitrary data available as a file.
type dataNode struct {
	nodefs.Node
	data []byte
}

func (d *dataNode) Open(flags uint32, content *fuse.Context) (nodefs.File, fuse.Status) {
	return nodefs.NewDataFile(d.data), fuse.OK
}

func newDataNode(c string) nodefs.Node {
	return &dataNode{nodefs.NewDefaultNode(), []byte(c)}
}

// NewGitilesRoot returns the root node for a file system.
func NewGitilesRoot(c *cache.Cache, tree *gitiles.Tree, service *gitiles.RepoService, options GitilesOptions) nodefs.Node {
	r := &gitilesRoot{
		Node:     nodefs.NewDefaultNode(),
		service:  service,
		cache:    c,
		shaMap:   map[git.Oid]string{},
		tree:     tree,
		opts:     options,
		lazyRepo: cache.NewLazyRepo(options.CloneURL, c),
	}

	return r
}

func (r *gitilesRoot) OnMount(fsConn *nodefs.FileSystemConnector) {
	if err := r.onMount(fsConn); err != nil {
		log.Printf("onMount: %v", err)
		for k := range r.Inode().Children() {
			r.Inode().RmChild(k)
		}
		r.Inode().NewChild("ERROR", false, newDataNode(err.Error()))
	}
}

func (r *gitilesRoot) onMount(fsConn *nodefs.FileSystemConnector) error {
	for _, e := range r.tree.Entries {
		if e.Type == "commit" {
			// TODO(hanwen): support submodules.
			continue
		}
		if e.Type != "blob" {
			log.Panicf("unexpected object type %s", e.Type)
		}

		p := e.Name
		dir, base := filepath.Split(p)

		parent, left := fsConn.Node(r.Inode(), dir)
		for _, l := range left {
			ch := parent.NewChild(l, true, nodefs.NewDefaultNode())
			parent = ch
		}
		id, err := git.NewOid(e.ID)
		if err != nil {
			return nil
		}

		// Determine if file should trigger a clone.
		clone := r.opts.CloneURL != ""
		for _, e := range r.opts.CloneOption {
			if e.RE.FindString(p) != "" {
				clone = e.Clone
				break
			}
		}

		n := &gitilesNode{
			Node:  nodefs.NewDefaultNode(),
			id:    *id,
			mode:  uint32(e.Mode),
			clone: clone,
			root:  r,
			mtime: time.Unix(0, 0),
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
	}

	// We don't need the tree data anymore.
	r.tree = nil
	return nil
}
