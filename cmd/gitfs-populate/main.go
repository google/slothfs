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

package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type repoTree struct {
	// repositories under this repository
	children map[string]*repoTree

	// files in this repository.
	entries []string
}

func newRepoTree(localRoot string) *repoTree {
	return &repoTree{
		children: make(map[string]*repoTree),
	}
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

// construct fills `parent` looking through `dir` subdir of `repoRoot`.
func construct(repoRoot, dir string, parent *repoTree) error {
	isRepo := false
	localRoot := filepath.Join(repoRoot, dir)
	if stat, err := os.Stat(filepath.Join(localRoot, ".git")); err == nil && stat.IsDir() {
		isRepo = true
	} else if stat, err := os.Stat(filepath.Join(localRoot, ".gitid")); err == nil && !stat.IsDir() {
		isRepo = true
	}

	if isRepo {
		sub := newRepoTree(localRoot)
		parent.children[dir] = sub
		parent = sub

		repoRoot = localRoot
		dir = ""
	}

	entries, err := ioutil.ReadDir(localRoot)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if (e.IsDir() && e.Name() == ".git") || (!e.IsDir() && e.Name() == ".gitid") {
			continue
		}

		subName := filepath.Join(dir, e.Name())
		if e.IsDir() {
			construct(repoRoot, subName, parent)
		} else {
			parent.entries = append(parent.entries, subName)
		}
	}
	return nil
}

// symlinkRepo creates symlinks for all the files in `child`.
func symlinkRepo(name string, child *repoTree, roRoot, rwRoot string) error {
	fi, err := os.Stat(filepath.Join(rwRoot, name))
	if err == nil && fi.IsDir() {
		return nil
	}

	for _, e := range child.entries {
		dest := filepath.Join(rwRoot, name, e)

		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := os.Symlink(filepath.Join(roRoot, name, e), dest); err != nil {
			return err
		}
	}
	return nil
}

// createTreeLinks tries to short-cut symlinks for whole trees by
// symlinking to the root of a repository in the RO tree.
func createTreeLinks(ro, rw *repoTree, roRoot, rwRoot string) error {
	allRW := rw.allChildren()

outer:
	for nm, ch := range ro.children {
		foundCheckout := false
		foundRecurse := false
		for k := range allRW {
			if k == "" {
				continue
			}
			if nm == k {
				foundRecurse = true
				break
			}
			rel, err := filepath.Rel(nm, k)
			if err != nil {
				return err
			}

			if strings.HasPrefix(rel, "..") {
				continue
			}

			// we have a checkout below "nm".
			foundCheckout = true
			break
		}

		switch {
		case foundRecurse:
			if err := createTreeLinks(ch, rw.children[nm], filepath.Join(roRoot, nm), filepath.Join(rwRoot, nm)); err != nil {
				return err
			}
			continue outer
		case !foundCheckout:
			dest := filepath.Join(rwRoot, nm)
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			if err := os.Symlink(filepath.Join(roRoot, nm), dest); err != nil {
				return err
			}
		}
	}
	return nil
}

// createLinks will populate a RW tree with symlinks to the RO tree.
func createLinks(ro, rw *repoTree, roRoot, rwRoot string) error {
	if err := createTreeLinks(ro, rw, roRoot, rwRoot); err != nil {
		return err
	}

	rwc := rw.allChildren()
	for nm, ch := range ro.allChildren() {
		if _, ok := rwc[nm]; !ok {
			if err := symlinkRepo(nm, ch, roRoot, rwRoot); err != nil {
				return err
			}
		}
	}
	return nil
}

// clearLinks removes all symlinks to the RO tree.
func clearLinks(dir, mount string) error {
	var dirs []string
	if err := filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(n)
			if err != nil {
				return err
			}
			if strings.HasPrefix(target, mount) {
				if err := os.Remove(n); err != nil {
					return err
				}
			}
		}
		if fi.IsDir() {
			dirs = append(dirs, n)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, d := range dirs {
		// Ignore error: dir may still contain entries.
		os.Remove(d)
	}
	return nil
}

// populateCheckout updates a RW dir with new symlinks to the given RO dir.
func populateCheckout(ro, rw string) error {
	if err := clearLinks(ro, rw); err != nil {
		log.Fatal(err)
	}

	rwTree := newRepoTree(rw)
	if err := construct(rw, "", rwTree); err != nil {
		return err
	}

	roTree := newRepoTree(ro)
	if err := construct(ro, "", roTree); err != nil {
		return err
	}

	return createLinks(roTree, rwTree, ro, rw)
}

func main() {
	mount := flag.String("ro", "", "path to gitfs-multifs mount.")
	flag.Parse()

	dir := "."
	if len(flag.Args()) == 1 {
		dir = flag.Arg(0)
	} else if len(flag.Args()) > 1 {
		log.Fatal("too many arguments.")
	}

	if err := populateCheckout(*mount, dir); err != nil {
		log.Fatalf("populateCheckout: %v", err)
	}
}
