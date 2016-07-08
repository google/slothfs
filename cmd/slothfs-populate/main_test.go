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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

const attr = "user.gitsha1"
const checksum = "3f75526aa8f01eea5d76cee10722195dc73676de"

func createFSTree(names []string) (string, error) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return dir, err
	}

	for _, f := range names {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return dir, err
		}
		if err := ioutil.WriteFile(p, []byte{42}, 0644); err != nil {
			return dir, err
		}
		if err := syscall.Setxattr(p, attr, []byte(checksum), 0); err != nil {
			return dir, fmt.Errorf("Setxattr: %v", err)
		}
	}
	return dir, nil
}

func TestConstruct(t *testing.T) {
	dir, err := createFSTree([]string{
		"toplevel",
		"build/core/.git/HEAD",
		"build/core/subdir/core.h",
		"build/core/song/.git/HEAD",
		"build/core/song/song.mp3",
		"build/core/top",
		"build/subfile",
	})
	if err != nil {
		t.Fatal(err)
	}
	songT := &repoTree{
		children: map[string]*repoTree{},
		entries: map[string]*fileInfo{
			"song.mp3": &fileInfo{isRegular: true, size: 1},
		},
	}
	coreT := &repoTree{
		children: map[string]*repoTree{"song": songT},
		entries: map[string]*fileInfo{
			"subdir/core.h": &fileInfo{isRegular: true, size: 1},
			"top":           &fileInfo{isRegular: true, size: 1},
		},
	}
	topT := &repoTree{
		children: map[string]*repoTree{
			"build/core": coreT,
		},
		entries: map[string]*fileInfo{
			"build/subfile": &fileInfo{isRegular: true, size: 1},
			"toplevel": &fileInfo{
				isRegular: true, size: 1},
		},
	}

	got, err := newRepoTree(dir)
	if err != nil {
		t.Fatalf("newRepoTree: %v", err)
	}

	// Clear unpredictable data.
	gotCh := got.allChildren()
	for _, t := range gotCh {
		for _, e := range t.entries {
			e.inode = 0
		}
	}

	wantCh := topT.allChildren()
	for k, v := range wantCh {
		if !reflect.DeepEqual(v, gotCh[k]) {
			t.Fatalf("subrepo %q: got %#v want %#v", v, gotCh[k])
		}
	}

	if !reflect.DeepEqual(got, topT) {
		t.Errorf("got %#v want %#v", got, topT)
	}
}

func TestPopulate(t *testing.T) {
	dir, err := createFSTree([]string{
		"ro/toplevel",
		"ro/build/core/.gitid",
		"ro/build/core/subdir/core.h",
		"ro/build/core/song/.gitid",
		"ro/build/core/song/song.mp3",
		"ro/build/core/top",
		"ro/build/subfile",
		"ro/platform/art/.gitid",
		"ro/platform/art/art.c",
		"ro/platform/art/art.h",
		"ro/platform/art/painting/.gitid",
		"ro/platform/art/painting/picasso.c",
		"rw/build/core/.git/head",
		"rw/build/core/newdir/bla",
	})
	if err != nil {
		t.Fatal("createFSTree:", err)
	}

	if err := os.Symlink(filepath.Join(dir, "ro/obsolete"), filepath.Join(dir, "rw/obsolete")); err != nil {
		t.Errorf("Symlink: %v", err)
	}

	if err := populateCheckout(filepath.Join(dir, "ro"), filepath.Join(dir, "rw")); err != nil {
		t.Errorf("populateCheckout: %v", err)
	}

	for _, f := range []string{
		"build/core/newdir/bla",
		"build/core/song/song.mp3",
		"platform/art/art.c",
		"platform/art/art.h",
		"platform/art/painting/picasso.c",
	} {
		fn := filepath.Join(dir, "rw", f)
		fi, err := os.Stat(fn)
		if err != nil || fi.Size() != 1 {
			t.Errorf("Stat(%s): %v, %v", fn, fi, err)
		}
	}

	// The following files are in repo that has a r/w checkout, so
	// they should not appear in the populated tree.
	for _, f := range []string{
		"ro/build/core/subdir/core.h",
		"ro/build/core/top",
	} {
		fn := filepath.Join(dir, "rw", f)
		_, err := os.Stat(fn)
		if err == nil {
			t.Errorf("file %s exists", fn)
		}
	}

	if fi, err := os.Lstat(filepath.Join(dir, "rw/obsolete")); err == nil {
		t.Fatalf("obsolete symlink still there: %v", fi)
	}
}

func TestChangedFiles(t *testing.T) {
	dir, err := createFSTree([]string{
		"r1/manifest.xml",
		"r1/same",
		"r1/checksum",
		"r1/size",
		"r2/manifest.xml",
		"r2/same",
		"r2/checksum",
		"r2/newfile",
		"r2/size",
	})
	if err != nil {
		t.Fatalf("createFSTree: %v", err)
	}

	if err := os.Symlink("same", filepath.Join(dir, "r2/symlink")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// same size, different checksum.
	ck2 := "3f75526aa8f01eea5d76cee10722195dc73676df"
	if err := syscall.Setxattr(filepath.Join(dir, "r2/checksum"), attr, []byte(ck2), 0); err != nil {
		t.Fatalf("Setxattr: %v", err)
	}

	// different size.
	if err := ioutil.WriteFile(filepath.Join(dir, "r2/size"), []byte("changed"), 0644); err != nil {
		t.Fatalf("WriteFile(%s/r2/size): %v", dir, err)
	}

	// Manifest should be ignored.
	if err := ioutil.WriteFile(filepath.Join(dir, "r2/manifest.xml"), []byte("changed"), 0644); err != nil {
		t.Fatalf("WriteFile(%s/r2/manifest): %v", dir, err)
	}

	r2tree, err := newRepoTree(filepath.Join(dir, "r2"))
	if err != nil {
		t.Fatalf("newRepoTree: %v", err)
	}
	r1tree, err := newRepoTree(filepath.Join(dir, "r1"))
	if err != nil {
		t.Fatalf("newRepoTree: %v", err)
	}

	oldRoot := filepath.Join(dir, "r1")
	got, err := changedFiles(oldRoot, r1tree.allFiles(), filepath.Join(dir, "r2"), r2tree.allFiles())
	if err != nil {
		t.Fatalf("changedFiles: %v", err)
	}
	if want := []string{"checksum", "newfile", "size"}; !reflect.DeepEqual(want, got) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestClearEmptyDirs(t *testing.T) {
	dir, err := createFSTree([]string{
		"ro/build/sub/sub2/p1/.gitid",
		"ro/build/sub/sub2/p1/build.mk",

		"rw/build/proj/.git/HEAD",
		"rw/build/proj/build.mk",

		"r3/toplevel",
	})
	if err != nil {
		t.Fatal("createFSTree:", err)
	}

	dest := filepath.Join(dir, "rw", "build/sub/sub2/p1")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Errorf("MkdirAll: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "ro", "build/sub/sub2/p1"), dest); err != nil {
		t.Errorf("Symlink(%s): %v", dest, err)
	}

	if err := populateCheckout(filepath.Join(dir, "r3"), filepath.Join(dir, "rw")); err != nil {
		t.Errorf("populateCheckout: %v", err)
	}

	gone := filepath.Join(dir, "rw", "build", "sub")
	if fi, err := os.Lstat(gone); err == nil {
		t.Errorf("directory %s still there: %v", gone, fi)
	}
}
