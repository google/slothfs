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

package cache

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/gitfs/gitiles"
	git "github.com/libgit2/git2go"
)

// A TreeCache caches recursively expanded trees by their git commit and tree IDs.
type TreeCache struct {
	dir string
}

// NewTreeCache constructs a new TreeCache.
func NewTreeCache(d string) (*TreeCache, error) {
	if err := os.MkdirAll(d, 0700); err != nil {
		return nil, err
	}
	return &TreeCache{dir: d}, nil
}

func (c *TreeCache) path(id *git.Oid) string {
	str := id.String()
	return fmt.Sprintf("%s/%s/%s", c.dir, str[:3], str[3:])
}

// Get returns a tree, if available.
func (c *TreeCache) Get(id *git.Oid) (*gitiles.Tree, error) {
	content, err := ioutil.ReadFile(c.path(id))
	if err != nil {
		return nil, err
	}
	var t gitiles.Tree
	if err := json.Unmarshal(content, &t); err != nil {
		return nil, err
	}

	return &t, nil
}

// Add adds a Tree to the cache
func (c *TreeCache) Add(id *git.Oid, tree *gitiles.Tree) error {
	if err := c.add(id, tree); err != nil {
		return err
	}

	if id.String() != tree.ID {
		treeID, err := git.NewOid(tree.ID)
		if err != nil {
			return err
		}
		return c.add(treeID, tree)
	}
	return nil
}

func (c *TreeCache) add(id *git.Oid, tree *gitiles.Tree) error {
	f, err := ioutil.TempFile(c.dir, "tmp")
	if err != nil {
		return err
	}

	content, err := json.Marshal(tree)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err

	}

	dir := filepath.Dir(c.path(id))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), c.path(id)); err != nil {
		return err
	}
	return nil
}

// GetTree loads the Tree from an on-disk Git repository.
func GetTree(repo *git.Repository, id *git.Oid) (*gitiles.Tree, error) {
	obj, err := repo.Lookup(id)
	if err != nil {
		return nil, err
	}
	defer obj.Free()

	peeledObj, err := obj.Peel(git.ObjectTree)
	if err != nil {
		return nil, err
	}
	defer peeledObj.Free()

	asTree, err := peeledObj.AsTree()
	if err != nil {
		return nil, err
	}

	var tree gitiles.Tree
	tree.ID = obj.Id().String()

	odb, err := repo.Odb()
	if err != nil {
		return nil, err
	}
	defer odb.Free()

	cb := func(n string, e *git.TreeEntry) int {
		t := ""
		var size *int
		switch e.Type {
		case git.ObjectTree:
			return 0
		case git.ObjectCommit:
			t = "commit"
		case git.ObjectBlob:
			t = "blob"
			sz, _, rhErr := odb.ReadHeader(e.Id)
			if rhErr != nil {
				err = rhErr
				return -1
			}
			size = new(int)
			*size = int(sz)

		default:
			err = fmt.Errorf("illegal object %d for %s", e.Type, n)
		}

		gEntry := gitiles.TreeEntry{
			Name: filepath.Join(n, e.Name),
			ID:   e.Id.String(),
			Mode: int(e.Filemode),
			Size: size,
			Type: t,
		}
		if e.Filemode == git.FilemodeLink {
			obj, lookErr := repo.Lookup(e.Id)
			if err != nil {
				err = lookErr
				return -1
			}
			defer obj.Free()

			blob, blobErr := obj.AsBlob()
			if blobErr != nil {
				err = blobErr
				return -1
			}

			target := string(blob.Contents())
			gEntry.Target = &target
		}

		tree.Entries = append(tree.Entries, gEntry)
		return 0
	}

	if err := asTree.Walk(cb); err != nil {
		return nil, err
	}

	return &tree, nil
}
