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

// Package populate holds the code to augment a partial R/W checkout
// with a symlink forest into a SlothFS workspace.
package populate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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

	for _, c := range ro.copied {
		if err := os.Symlink(filepath.Join(roRoot, c), filepath.Join(rwRoot, c)); err != nil && !os.IsExist(err) {
			return err
		}
	}

	return nil
}

// clearLinks removes all symlinks to the RO tree. It returns the workspace names that were linked before.
func clearLinks(mount, dir string) (map[string]struct{}, error) {
	mount = filepath.Clean(mount)

	var dirs []string

	prevPrefixes := map[string]struct{}{}
	if err := filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
		if fi == nil {
			return fmt.Errorf("Walk %s: nil fileinfo for %s", dir, n)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(n)
			if err != nil {
				return err
			}
			if strings.HasPrefix(target, mount) {
				prevPrefixes[trimMount(target, mount)] = struct{}{}
				if err := os.Remove(n); err != nil {
					return err
				}
			}
		}
		if fi.IsDir() && n != dir {
			dirs = append(dirs, n)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("Walk %s: %v", dir, err)
	}

	sort.Strings(dirs)
	for i := range dirs {
		// Reverse the ordering, so we get the deepest subdirs first.
		d := dirs[len(dirs)-1-i]
		// Ignore error: dir may still contain entries.
		os.Remove(d)
	}

	return prevPrefixes, nil
}

func trimMount(dir, mount string) string {
	dir = strings.TrimPrefix(dir, mount+"/")
	if i := strings.Index(dir, "/"); i != -1 {
		dir = dir[:i]
	}

	return dir
}

// Returns the filenames (as relative paths) in newDir that have
// changed relative to the files in oldDir.
func changedFiles(oldInfos map[string]*fileInfo, newInfos map[string]*fileInfo) (added, changed []string, err error) {
	for path, info := range newInfos {
		old, ok := oldInfos[path]
		if !ok {
			added = append(added, path)
			continue
		}

		if old.sha1 == nil || info.sha1 == nil {
			changed = append(changed, path)
			continue
		}
		if bytes.Compare(old.sha1[:], info.sha1[:]) != 0 {
			changed = append(changed, path)
			continue
		}
	}
	sort.Strings(changed)
	sort.Strings(added)
	return added, changed, nil
}

// Checkout updates a RW dir with new symlinks to the given RO dir.
// Returns the files that should be touched.
func Checkout(ro, rw string) (added, changed []string, err error) {
	ro = filepath.Clean(ro)
	wsNames, err := clearLinks(filepath.Dir(ro), rw)
	if err != nil {
		return nil, nil, err
	}

	oldRoot := ""
	for nm := range wsNames {
		r := filepath.Join(filepath.Dir(ro), nm)
		if _, err := os.Stat(r); err == nil && r != ro {
			oldRoot = r
			break
		}
	}

	// Do the file system traversals in parallel.
	errs := make(chan error, 3)
	var rwTree, roTree *repoTree
	var oldInfos map[string]*fileInfo

	if oldRoot != "" {
		go func() {
			t, err := repoTreeFromSlothFS(oldRoot)
			if t != nil {
				oldInfos = t.allFiles()
			}
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
		t, err := repoTreeFromSlothFS(ro)
		roTree = t
		errs <- err
	}()

	for i := 0; i < cap(errs); i++ {
		err := <-errs
		if err != nil {
			return nil, nil, err
		}
	}

	if err := createLinks(roTree, rwTree, ro, rw); err != nil {
		return nil, nil, err
	}

	newInfos := roTree.allFiles()
	added, changed, err = changedFiles(oldInfos, newInfos)
	if err != nil {
		return nil, nil, fmt.Errorf("changedFiles: %v", err)
	}

	for i, p := range changed {
		changed[i] = filepath.Join(ro, p)
	}

	for i, p := range added {
		added[i] = filepath.Join(ro, p)
	}

	return added, changed, nil
}
