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

package populate

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/manifest"
)

// fileInfo holds data files contained in the git repository within a
// repoTree node.
type fileInfo struct {
	// the SHA1 of the file. This can be nil if getting it was too expensive.
	sha1 *plumbing.Hash
}

// repoTree is a nested set of Git repositories.
type repoTree struct {
	// repositories under this repository
	children map[string]*repoTree

	// files in this repository.
	entries map[string]*fileInfo

	// paths that are instantiated with Copyfile or Linkfile.
	copied []string
}

// findParentRepo recursively finds the deepest child that is a prefix
// to the given path.
func (t *repoTree) findParentRepo(path string) (*repoTree, string) {
	for k, ch := range t.children {
		if strings.HasPrefix(path, k+"/") {
			return ch.findParentRepo(path[len(k+"/"):])
		}
	}
	return t, path
}

// write dumps the tree for debugging purposes.
func (t *repoTree) write(w io.Writer, indent string) {
	for nm, ch := range t.children {
		fmt.Fprintf(w, "%s%s:\n", indent, nm)
		ch.write(w, indent+" ")
	}
}

// repoTreeFromManifest creates a repoTree from a manifest XML.
func repoTreeFromManifest(xmlFile string) (*repoTree, error) {
	mf, err := manifest.ParseFile(xmlFile)
	if err != nil {
		return nil, err
	}

	var byDepth [][]*manifest.Project
	for i, p := range mf.Project {
		l := len(strings.Split(p.GetPath(), "/"))
		for len(byDepth) <= l {
			byDepth = append(byDepth, nil)
		}

		byDepth[l] = append(byDepth[l], &mf.Project[i])
	}

	root := makeRepoTree()
	treesByPath := map[string]*repoTree{
		"": root,
	}

	for _, projs := range byDepth {
		for _, p := range projs {
			childTree := makeRepoTree()
			treesByPath[p.GetPath()] = childTree

			parent, key := root.findParentRepo(p.GetPath())
			parent.children[key] = childTree
		}
	}

	for _, p := range mf.Project {
		for _, c := range p.Copyfile {
			root.copied = append(root.copied, c.Dest)
		}
		for _, c := range p.Linkfile {
			root.copied = append(root.copied, c.Dest)
		}
	}
	sort.Strings(root.copied)
	return root, nil
}

// fillFromSlothFS reads tree.json to fill Entries for this repoTree
// node only, and does not recurse.
func (t *repoTree) fillFromSlothFS(dir string) error {
	c, err := ioutil.ReadFile(filepath.Join(dir, ".slothfs", "tree.json"))
	if err != nil {
		return err
	}

	var tree gitiles.Tree
	if err := json.Unmarshal(c, &tree); err != nil {
		return err
	}

	for _, e := range tree.Entries {
		fi := &fileInfo{}
		fi.sha1, err = parseID(e.ID)
		if err != nil {
			return err
		}

		t.entries[e.Name] = fi
	}

	return nil
}

// repoTreeFromSlothFS reads data from .slothfs to construct a fully
// populated repoTree tree.
func repoTreeFromSlothFS(dir string) (*repoTree, error) {
	root, err := repoTreeFromManifest(filepath.Join(dir, ".slothfs", "manifest.xml"))
	if err != nil {
		return nil, err
	}

	chs := root.allChildren()
	errs := make(chan error, len(chs))
	for path, ch := range root.allChildren() {
		go func(p string, t *repoTree) {
			err := t.fillFromSlothFS(p)
			errs <- err
		}(filepath.Join(dir, path), ch)
	}

	for i := 0; i < cap(errs); i++ {
		err := <-errs
		if err != nil {
			return nil, err
		}
	}
	return root, nil
}

// makeRepoTree returns a repoTree struct with maps initialized.
func makeRepoTree() *repoTree {
	return &repoTree{
		children: map[string]*repoTree{},
		entries:  map[string]*fileInfo{},
	}
}

// newRepoTree returns a repoTree constructed from filesystem data.
func newRepoTree(dir string) (*repoTree, error) {
	t := makeRepoTree()
	if err := t.fill(dir, ""); err != nil {
		return nil, err
	}
	return t, nil
}

// allChildren returns all the repositories (including the receiver)
// as a map keyed by relative path.
func (t *repoTree) allChildren() map[string]*repoTree {
	r := map[string]*repoTree{"": t}
	for nm, ch := range t.children {
		for sub, subCh := range ch.allChildren() {
			r[filepath.Join(nm, sub)] = subCh
		}
	}
	return r
}

// allFiles returns all the files below this repoTree.
func (t *repoTree) allFiles() map[string]*fileInfo {
	r := map[string]*fileInfo{}
	for nm, info := range t.entries {
		r[nm] = info
	}
	for nm, ch := range t.children {
		for sub, subCh := range ch.allFiles() {
			r[filepath.Join(nm, sub)] = subCh
		}
	}
	return r
}

// returns whether path is the topdirectory of some git repository,
// either in plain git or in slothfs.
func isRepoDir(path string) bool {
	if stat, err := os.Stat(filepath.Join(path, ".git")); err == nil && stat.IsDir() {
		return true
	} else if stat, err := os.Stat(filepath.Join(path, ".slothfs")); err == nil && stat.IsDir() {
		return true
	}
	return false
}

// construct fills `parent` looking through `dir` subdir of `repoRoot`.
func (t *repoTree) fill(repoRoot, dir string) error {
	entries, err := ioutil.ReadDir(filepath.Join(repoRoot, dir))
	if err != nil {
		log.Println(repoRoot, err)
		return err
	}

	todo := map[string]*repoTree{}
	for _, e := range entries {
		if e.IsDir() && (e.Name() == ".git" || e.Name() == ".slothfs") {
			continue
		}
		if e.IsDir() && e.Name() == "out" && dir == "" {
			// Ignore the build output directory.
			continue
		}

		subName := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if newRoot := filepath.Join(repoRoot, subName); isRepoDir(newRoot) {
				ch := makeRepoTree()
				t.children[subName] = ch
				todo[newRoot] = ch
			} else {
				t.fill(repoRoot, subName)
			}
		} else {
			t.entries[subName] = &fileInfo{}
		}
	}

	errs := make(chan error, len(todo))
	for newRoot, ch := range todo {
		go func(r string, t *repoTree) {
			errs <- t.fill(r, "")
		}(newRoot, ch)
	}

	for range todo {
		err := <-errs
		if err != nil {
			return err
		}
	}

	return nil
}
