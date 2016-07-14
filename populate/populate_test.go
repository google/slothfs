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
			"song.mp3": &fileInfo{},
		},
	}
	coreT := &repoTree{
		children: map[string]*repoTree{"song": songT},
		entries: map[string]*fileInfo{
			"subdir/core.h": &fileInfo{},
			"top":           &fileInfo{},
		},
	}
	topT := &repoTree{
		children: map[string]*repoTree{
			"build/core": coreT,
		},
		entries: map[string]*fileInfo{
			"build/subfile": &fileInfo{},
			"toplevel":      &fileInfo{},
		},
	}

	got, err := newRepoTree(dir)
	if err != nil {
		t.Fatalf("newRepoTree: %v", err)
	}

	// Clear unpredictable data.
	gotCh := got.allChildren()

	wantCh := topT.allChildren()
	for k, v := range wantCh {
		if !reflect.DeepEqual(v, gotCh[k]) {
			t.Fatalf("subrepo %q: got %#v want %#v", k, gotCh[k], v)
		}
	}

	if !reflect.DeepEqual(got, topT) {
		t.Errorf("got %#v want %#v", got, topT)
	}
}

func TestRepoTreeFromManifest(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal("TempFile", err)
	}

	_, err = f.Write([]byte(`
<Manifest>
 <default revision="master" remote="aosp" dest-branch="" sync-j="4" sync-c="" sync-s=""></default>
 <remote alias="" name="aosp" fetch=".." review="https://android-review.googlesource.com/" revision=""></remote>
 <project path="build" name="platform/build" groups="pdk,tradefed" revision="55d4a46f6da08b248a467097d56a2762d47d7043" clone-url="https://android.googlesource.com/platform/build">
  <copyfile src="core/root.mk" dest="Makefile"></copyfile>
 </project>
 <project path="build/blueprint" name="platform/build/blueprint" groups="pdk,tradefed" revision="f0de34718cb9dcb6fbe3bb3afb2a1ef4eae85118" clone-url="https://android.googlesource.com/platform/build/blueprint">
  <linkfile src="root.bp" dest="Android.bp"></linkfile>
 </project>
 <project path="build/subdir/kati" name="platform/build/subdir/kati" groups="pdk,tradefed" revision="ff2d59e2e082d17ae04f43d409244440a1687856" clone-url="https://android.googlesource.com/platform/build/subdir/kati"></project>
</Manifest>`))
	if err != nil {
		t.Fatal("Write", err)
	}

	blueprintT := &repoTree{
		children: map[string]*repoTree{},
		entries:  map[string]*fileInfo{},
	}
	katiT := &repoTree{
		children: map[string]*repoTree{},
		entries:  map[string]*fileInfo{},
	}
	buildT := &repoTree{
		children: map[string]*repoTree{
			"subdir/kati": katiT,
			"blueprint":   blueprintT,
		},
		entries: map[string]*fileInfo{},
	}
	topT := &repoTree{
		children: map[string]*repoTree{
			"build": buildT,
		},
		entries: map[string]*fileInfo{},
		copied: []string{
			"Android.bp",
			"Makefile",
		},
	}

	got, err := repoTreeFromManifest(f.Name())
	if err != nil {
		t.Fatalf("repoTreeFromManifest: %v", err)
	}
	if !reflect.DeepEqual(got, topT) {
		t.Errorf("got %#v, want %#v", got, topT)
	}
}
