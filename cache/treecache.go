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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/slothfs/gitiles"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/filemode"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	git "gopkg.in/src-d/go-git.v4"
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

func (c *TreeCache) path(id *plumbing.Hash) string {
	str := id.String()
	return fmt.Sprintf("%s/%s/%s", c.dir, str[:3], str[3:])
}

// Get returns a tree, if available.
func (c *TreeCache) Get(id *plumbing.Hash) (*gitiles.Tree, error) {
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

func parseID(s string) (*plumbing.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 20 {
		return nil, fmt.Errorf("NewOid(%q): %v", s, err)
	}

	var h plumbing.Hash
	copy(h[:], b)
	return &h, nil
}

// Add adds a Tree to the cache
func (c *TreeCache) Add(id *plumbing.Hash, tree *gitiles.Tree) error {
	if err := c.add(id, tree); err != nil {
		return err
	}

	if id.String() != tree.ID {
		// Ugh: error handling?
		treeID, err := parseID(tree.ID)
		if err != nil {
			return err
		}
		return c.add(treeID, tree)
	}
	return nil
}

func (c *TreeCache) add(id *plumbing.Hash, tree *gitiles.Tree) error {
	f, err := ioutil.TempFile(c.dir, "tmp")
	if err != nil {
		return err
	}

	content, err := json.MarshalIndent(tree, "", " ")
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
func GetTree(repo *git.Repository, id *plumbing.Hash) (*gitiles.Tree, error) {
	treeObj, err := repo.TreeObject(*id)
	if treeObj == nil {
		commit, e2 := repo.CommitObject(*id)
		if e2 != nil {
			return nil, e2
		}
		treeObj, err = repo.TreeObject(commit.TreeHash)
	}
	if err != nil {
		return nil, err
	}

	var tree gitiles.Tree

	tree.ID = id.String()
	walker := object.NewTreeWalker(treeObj, true, map[plumbing.Hash]bool{})
	defer walker.Close()
loop:
	for {
		name, entry, err := walker.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		var size *int
		var t string
		var blob *object.Blob
		switch entry.Mode {
		case filemode.Dir:
			continue loop
		case filemode.Submodule:
			t = "commit"
		case filemode.Symlink, filemode.Regular, filemode.Executable:
			t = "blob"
			blob, err = repo.BlobObject(entry.Hash)
			if err != nil {
				return nil, err
			}
			size = new(int)
			*size = int(blob.Size)
		default:
			err = fmt.Errorf("illegal mode %d for %s", entry.Mode, name)
		}

		gEntry := gitiles.TreeEntry{
			Name: name,
			ID:   entry.Hash.String(),
			Mode: int(entry.Mode),
			Size: size,
			Type: t,
		}
		if entry.Mode == filemode.Symlink {
			r, err := blob.Reader()
			if err != nil {
				return nil, err
			}
			c, err := ioutil.ReadAll(r)
			r.Close()
			if err != nil {
				return nil, err
			}
			target := string(c)
			gEntry.Target = &target
		}

		tree.Entries = append(tree.Entries, gEntry)
	}

	return &tree, nil
}
