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
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	git "github.com/libgit2/git2go"
)

type fileInfo struct {
	isRegular bool
	size      int64
	inode     uint64
}

type repoTree struct {
	// repositories under this repository
	children map[string]*repoTree

	// files in this repository.
	entries map[string]*fileInfo
}

func makeRepoTree() *repoTree {
	return &repoTree{
		children: map[string]*repoTree{},
		entries:  map[string]*fileInfo{},
	}
}

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

func isRepoDir(path string) bool {
	if stat, err := os.Stat(filepath.Join(path, ".git")); err == nil && stat.IsDir() {
		return true
	} else if stat, err := os.Stat(filepath.Join(path, ".gitid")); err == nil && !stat.IsDir() {
		return true
	}
	return false
}

// construct fills `parent` looking through `dir` subdir of `repoRoot`.
func (parent *repoTree) fill(repoRoot, dir string) error {
	entries, err := ioutil.ReadDir(filepath.Join(repoRoot, dir))
	if err != nil {
		return err
	}

	todo := map[string]*repoTree{}
	for _, e := range entries {
		if (e.IsDir() && e.Name() == ".git") || (!e.IsDir() && e.Name() == ".gitid") {
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
				parent.children[subName] = ch
				todo[newRoot] = ch
			} else {
				parent.fill(repoRoot, subName)
			}
		} else {
			parent.entries[subName] = &fileInfo{
				isRegular: e.Mode()&os.ModeType == 0,
				size:      e.Size(),
				inode:     e.Sys().(*syscall.Stat_t).Ino,
			}
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

// symlinkRepo creates symlinks for all the files in `child`.
func symlinkRepo(name string, child *repoTree, roRoot, rwRoot string) error {
	fi, err := os.Stat(filepath.Join(rwRoot, name))
	if err == nil && fi.IsDir() {
		return nil
	}

	for e := range child.entries {
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

// clearLinks removes all symlinks to the RO tree. It returns the workspace name that was linked before.
func clearLinks(mount, dir string) (string, error) {
	mount = filepath.Clean(mount)

	var prefix string
	var dirs []string
	if err := filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(n)
			if err != nil {
				return err
			}
			if strings.HasPrefix(target, mount) {
				prefix = target
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
		return "", err
	}

	// Reverse the ordering, so we get the deepest subdirs first.
	sort.Strings(dirs)
	for i := range dirs {
		d := dirs[len(dirs)-1-i]
		// Ignore error: dir may still contain entries.
		os.Remove(d)
	}

	prefix = strings.TrimPrefix(prefix, mount+"/")
	if i := strings.Index(prefix, "/"); i != -1 {
		prefix = prefix[:i]
	}
	return prefix, nil
}

const attrName = "user.gitsha1"

func getSHA1(fn string) (*git.Oid, error) {
	var data [40]byte
	sz, err := syscall.Getxattr(fn, attrName, data[:])
	if err != nil {
		return nil, fmt.Errorf("Getxattr(%s, %s): %v", fn, attrName, err)
	}

	oid, err := git.NewOid(string(data[:sz]))
	if err != nil {
		return nil, err
	}
	return oid, nil
}

// Returns the filenames (as relative paths) in newDir that have
// changed relative to the files in oldDir.
func changedFiles(oldDir string, oldInfos map[string]*fileInfo,
	newDir string, newInfos map[string]*fileInfo) ([]string, error) {
	var changed []string
	for path, info := range newInfos {
		if path == "manifest.xml" {
			continue
		}

		if !info.isRegular {
			// TODO(hanwen): this is incorrect. If a file
			// changes from a blob to a symlink, we should
			// deref the symlink and check if the blob has
			// changed.
			continue
		}

		old, ok := oldInfos[path]
		if !ok {
			changed = append(changed, path)
			continue
		}

		if old.inode == info.inode {
			continue
		}

		if old.size != info.size {
			changed = append(changed, path)
			continue
		}

		oldSHA1, err := getSHA1(filepath.Join(oldDir, path))
		if err != nil {
			return nil, err
		}
		newSHA1, err := getSHA1(filepath.Join(newDir, path))
		if err != nil {
			return nil, err
		}

		if bytes.Compare(oldSHA1[:], newSHA1[:]) != 0 {
			changed = append(changed, path)
		}
	}

	sort.Strings(changed)
	return changed, nil
}

// populateCheckout updates a RW dir with new symlinks to the given RO dir.
func populateCheckout(ro, rw string) error {
	ro = filepath.Clean(ro)
	wsName, err := clearLinks(filepath.Dir(ro), rw)
	if err != nil {
		return err
	}
	oldRoot := filepath.Join(filepath.Dir(ro), wsName)

	// Do the file system traversals in parallel.
	errs := make(chan error, 3)
	var rwTree, roTree *repoTree
	var oldInfos map[string]*fileInfo

	if wsName != "" {
		go func() {
			t, err := newRepoTree(oldRoot)
			oldInfos = t.allFiles()
			errs <- err
		}()
	} else {
		oldInfos = map[string]*fileInfo{}
		errs <- nil
	}

	go func() {
		t, err := newRepoTree(rw)
		rwTree = t
		errs <- err
	}()
	go func() {
		t, err := newRepoTree(ro)
		roTree = t
		errs <- err
	}()

	for i := 0; i < cap(errs); i++ {
		err := <-errs
		if err != nil {
			return err
		}
	}

	if err := createLinks(roTree, rwTree, ro, rw); err != nil {
		return err
	}

	changed, err := changedFiles(oldRoot, oldInfos, ro, roTree.allFiles())
	if err != nil {
		return fmt.Errorf("changedFiles: %v", err)
	}

	for i, p := range changed {
		changed[i] = filepath.Join(ro, p)
	}

	if err := seqTouch(changed, time.Now()); err != nil {
		return err
	}

	return nil
}

func seqTouch(fs []string, t time.Time) error {
	for _, f := range fs {
		if err := os.Chtimes(f, t, t); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	mount := flag.String("ro", "", "path to slothfs-multifs mount.")
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
