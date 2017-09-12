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
	"encoding/hex"
	"fmt"
	"log"

	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/gitiles"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

type gitilesConfigFSRoot struct {
	nodefs.Node

	fsConn  *nodefs.FileSystemConnector
	cache   *cache.Cache
	service *gitiles.RepoService
	options GitilesOptions
}

func (r *gitilesConfigFSRoot) OnMount(fsConn *nodefs.FileSystemConnector) {
	r.fsConn = fsConn
}

func parseID(s string) (*plumbing.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 20 {
		return nil, fmt.Errorf("NewOid(%q): %v", s, err)
	}

	var h plumbing.Hash
	copy(h[:], b)
	return &h, nil
}

func (r *gitilesConfigFSRoot) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	id, err := parseID(name)
	if err != nil {
		return nil, fuse.ENOENT
	}

	if ch := r.Inode().GetChild(name); ch != nil {
		return ch, fuse.OK
	}

	tree, err := r.cache.Tree.Get(id)
	if err != nil {
		tree, err = r.service.GetTree(id.String(), "/", true)
		if err != nil {
			log.Printf("GetTree(%s): %v", id, err)
			return nil, fuse.EIO
		}

		if err := r.cache.Tree.Add(id, tree); err != nil {
			log.Printf("TreeCache.Add(%s): %v", id, err)
		}
	}

	gro := GitilesRevisionOptions{
		Revision:       id.String(),
		GitilesOptions: r.options,
	}
	newRoot := NewGitilesRoot(r.cache, tree, r.service, gro)
	ch := r.Inode().NewChild(id.String(), true, newRoot)
	out.Mode = fuse.S_IFDIR | 0755

	newRoot.OnMount(r.fsConn)
	return ch, fuse.OK
}

// NewGitilesConfigFSRoot returns a root node for a filesystem that lazily
// instantiates a repository if you access any subdirectory named by a
// 40-byte hex SHA1.
func NewGitilesConfigFSRoot(c *cache.Cache, service *gitiles.RepoService, options *GitilesOptions) nodefs.Node {
	// TODO(hanwen): nodefs.Node has an OnForget(), but it will
	// never trigger for directories that have children. That
	// means that we effectively never drop old trees. We can fix
	// this by either: 1) reconsidering OnForget in go-fuse 2) do
	// a periodic removal of all subtrees trees. Since the FS is
	// read-only that should cause no ill effects.
	return &gitilesConfigFSRoot{
		Node:    nodefs.NewDefaultNode(),
		cache:   c,
		service: service,
		options: *options,
	}
}
