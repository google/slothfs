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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/manifest"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

func newManifestTestFixture(mf *manifest.Manifest) (*testFixture, error) {
	fix, err := newTestFixture()
	if err != nil {
		return nil, err
	}

	opts := ManifestOptions{
		Manifest: mf,
	}

	fs, err := NewManifestFS(fix.service, fix.cache, opts)
	if err != nil {
		return nil, err
	}
	if err := fix.mount(fs); err != nil {
		return nil, err
	}

	return fix, nil
}

func TestManifestFSGitRepoSeedsTreeCache(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	// Add a git repo.
	cmd := exec.Command("/bin/sh", "-c",
		strings.Join([]string{
			"mkdir -p localhost/platform/build/kati.git",
			"cd localhost/platform/build/kati.git",
			"git init",
			"touch file",
			"git add file",
			"git commit -m msg -a",
			"git show --no-patch --pretty=format:'%H' HEAD | head",
		}, " && "))
	cmd.Dir = filepath.Join(fix.dir, "cache", "git")

	headSHA1 := ""
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create repo: %v, out: %s", err, string(out))
	} else {
		lines := strings.Split(string(out), "\n")
		headSHA1 = lines[len(lines)-1]
	}

	p := testManifest.Project[0]
	p.CloneURL = "file:///localhost/platform/build/kati"
	p.Revision = headSHA1

	mf := *testManifest
	mf.Project = nil
	mf.Project = append(mf.Project, p)

	opts := ManifestOptions{
		Manifest: &mf,
	}

	fs, err := NewManifestFS(fix.service, fix.cache, opts)
	if err != nil {
		t.Fatal("NewManifestFS", err)
	}
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	headID, err := parseID(headSHA1)
	if err != nil {
		t.Fatalf("parseID(%s): %v", headSHA1, err)
	}

	tree, err := fix.cache.Tree.Get(headID)
	if err != nil {
		t.Fatalf("treeCache.Get(%s): %v", headSHA1, err)
	}

	newInt := func(n int) *int { return &n }
	tree.ID = ""
	want := &gitiles.Tree{
		Entries: []gitiles.TreeEntry{
			{
				Name: "file",
				Mode: 0100644,
				Type: "blob",
				ID:   "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391",
				Size: newInt(0),
			},
		},
	}
	if !reflect.DeepEqual(tree, want) {
		t.Errorf("got cached tree %v, want %v", tree, want)
	}
}

func TestManifestFSCloneOption(t *testing.T) {
	mf := *testManifest
	for i := range mf.Project {
		mf.Project[i].CloneDepth = "1"
	}

	fix, err := newManifestTestFixture(&mf)
	if err != nil {
		t.Fatalf("newManifestTestFixture: %v", err)
	}
	defer fix.cleanup()

	fs := fix.root.(*manifestFSRoot)
	ch := fs.Inode()
	for _, n := range []string{"build", "kati", "AUTHORS"} {
		newCh := ch.GetChild(n)
		if ch == nil {
			t.Fatalf("node for %q not found. Have %s", n, ch.Children())
		}
		ch = newCh
	}

	giNode, ok := ch.Node().(*gitilesNode)
	if !ok {
		t.Fatalf("got node type %T, want *gitilesNode", ch.Node())
	}

	if giNode.clone {
		t.Errorf("file had clone set.")
	}
}

func TestManifestFSTimestamps(t *testing.T) {
	fix, err := newManifestTestFixture(testManifest)
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	var zeroFiles []string
	if err := filepath.Walk(fix.mntDir, func(n string, fi os.FileInfo, err error) error {
		if fi != nil && fi.ModTime().UnixNano() == 0 {
			r, _ := filepath.Rel(fix.mntDir, n)
			zeroFiles = append(zeroFiles, r)
		}
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(zeroFiles) > 0 {
		sort.Strings(zeroFiles)
		t.Errorf("found files with zero timestamps: %v", zeroFiles)
	}
}

func TestManifestFSBasic(t *testing.T) {
	fix, err := newManifestTestFixture(testManifest)
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	fn := filepath.Join(fix.mntDir, "build", "kati", "AUTHORS")
	fi, err := os.Lstat(fn)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", fn, err)
	}
	if fi.Size() != 373 {
		t.Errorf("got size %d want %d", fi.Size(), 373)
	}

	contents, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", fn, err)
	}

	want := testGitiles["/platform/build/kati/+show/ce34badf691d36e8048b63f89d1a86ee5fa4325c/AUTHORS?format=TEXT"]
	if string(contents) != want {
		t.Fatalf("got %q, want %q", contents, want)
	}

	copyPath := filepath.Join(fix.mntDir, "build", "copydest")
	if copyFI, err := os.Lstat(copyPath); err != nil {
		t.Errorf("Lstat(%s): %v", copyPath, err)
	} else {
		copyStat := copyFI.Sys().(*syscall.Stat_t)
		origStat := fi.Sys().(*syscall.Stat_t)

		if !reflect.DeepEqual(copyStat, origStat) {
			t.Errorf("got stat %v, want %v", copyStat, origStat)
		}
	}

	linkPath := filepath.Join(fix.mntDir, "build", "linkdest")
	if got, err := os.Readlink(linkPath); err != nil {
		t.Errorf("Readlink(%s): %v", linkPath, err)
	} else if want := "kati/AUTHORS"; got != want {
		t.Errorf("Readlink(%s) = %q, want %q", linkPath, got, want)
	}
}

func TestManifestFSXMLFile(t *testing.T) {
	fix, err := newManifestTestFixture(testManifest)
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	xmlPath := filepath.Join(fix.mntDir, ".slothfs", "manifest.xml")
	fuseMF, err := manifest.ParseFile(xmlPath)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", xmlPath, err)
	}

	if !reflect.DeepEqual(fuseMF, testManifest) {
		t.Errorf("read back manifest %v, want %v", fuseMF, testManifest)
	}
}

type testFixture struct {
	dir        string
	mntDir     string
	server     *fuse.Server
	cache      *cache.Cache
	testServer *testServer
	service    *gitiles.Service
	root       nodefs.Node
}

func (f *testFixture) cleanup() {
	if f.testServer != nil {
		f.testServer.listener.Close()
	}
	if f.server != nil {
		f.server.Unmount()
	}
	os.RemoveAll(f.dir)
}

func newTestFixture() (*testFixture, error) {
	d, err := ioutil.TempDir("", "slothfs")
	if err != nil {
		return nil, err
	}

	fixture := &testFixture{dir: d}

	fixture.cache, err = cache.NewCache(filepath.Join(d, "/cache"), cache.Options{})
	if err != nil {
		return nil, err
	}

	fixture.testServer, err = newTestServer()
	if err != nil {
		return nil, err
	}

	fixture.service, err = gitiles.NewService(gitiles.Options{
		Address: fmt.Sprintf("http://%s", fixture.testServer.addr),
	})
	if err != nil {
		return nil, err
	}

	return fixture, nil
}

func (f *testFixture) mount(root nodefs.Node) error {
	f.mntDir = filepath.Join(f.dir, "mnt")
	if err := os.Mkdir(f.mntDir, 0755); err != nil {
		return err
	}

	fuseOpts := &nodefs.Options{
		EntryTimeout:    time.Hour,
		NegativeTimeout: time.Hour,
		AttrTimeout:     time.Hour,
	}

	var err error
	f.server, _, err = nodefs.MountRoot(f.mntDir, root, fuseOpts)
	if err != nil {
		return err
	}

	if fuseDebug {
		f.server.SetDebug(true)
	}
	go f.server.Serve()

	f.root = root
	return nil
}

func TestMultiManifestFSBrokenXML(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatalf("newTestFixture: %v", err)
	}
	defer fix.cleanup()

	brokenXMLFile := filepath.Join(fix.dir, "broken.xml")
	if err := ioutil.WriteFile(brokenXMLFile, []byte("I'm not XML."), 0644); err != nil {
		t.Errorf("WriteFile(%s): %v", brokenXMLFile, err)
	}

	opts := MultiManifestFSOptions{}
	fs := NewMultiManifestFS(fix.service, fix.cache, opts)

	if err := fix.mount(fs); err != nil {
		t.Fatalf("mount: %v", err)
	}

	if err := os.Symlink(brokenXMLFile, filepath.Join(fix.mntDir, "config", "ws")); err == nil {
		t.Fatalf("want error for broken XML file")
	}
}

func TestMultiManifestFSBasic(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatalf("newTestFixture: %v", err)
	}
	defer fix.cleanup()

	xmlFile := filepath.Join(fix.dir, "manifest.xml")
	if err := ioutil.WriteFile(xmlFile, []byte(testManifestXML), 0644); err != nil {
		t.Errorf("WriteFile(%s): %v", xmlFile, err)
	}

	opts := MultiManifestFSOptions{}
	fs := NewMultiManifestFS(fix.service, fix.cache, opts)

	if err := fix.mount(fs); err != nil {
		t.Fatalf("mount: %v", err)
	}

	wsDir := filepath.Join(fix.mntDir, "ws")
	if fi, err := os.Lstat(wsDir); err == nil {
		t.Fatalf("got %v, want non-existent workspace dir", fi)
	}

	configName := filepath.Join(fix.mntDir, "config", "ws")
	if err := os.Symlink(xmlFile, configName); err != nil {
		t.Fatalf("Symlink(%s):  %v", xmlFile, err)
	}

	if _, err := os.Lstat(wsDir); err != nil {
		t.Fatalf("Lstat(%s): %v", wsDir, err)
	}

	if got, err := os.Readlink(configName); err != nil {
		t.Fatalf("Readlink(%s): %v", configName, err)
	} else if want := "../ws/.slothfs/manifest.xml"; got != want {
		t.Errorf("got link %s, want %s", got, want)
	}

	if _, err := manifest.ParseFile(configName); err != nil {
		t.Fatalf("ParseFile(%s): %v", configName, err)
	}

	fn := filepath.Join(wsDir, "build", "kati", "AUTHORS")
	if fi, err := os.Lstat(fn); err != nil {
		t.Fatalf("Lstat(%s): %v", fn, err)
	} else if fi.Size() != 373 {
		t.Errorf("got %d, want size 373", fi.Size())
	}

	if err := os.Remove(configName); err != nil {
		t.Fatalf("Delete(%s): %v", configName, err)
	}

	if fi, err := os.Lstat(wsDir); err == nil {
		t.Errorf("Lstat(%s): got %v, want error", wsDir, fi)
	}
}

func TestMultiManifestFSManifestDir(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatalf("newTestFixture: %v", err)
	}
	defer fix.cleanup()

	mfDir := filepath.Join(fix.dir, "manifests")
	if err := os.MkdirAll(mfDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	xmlFile := filepath.Join(mfDir, "ws")
	if err := ioutil.WriteFile(xmlFile, []byte(testManifestXML), 0644); err != nil {
		t.Errorf("WriteFile(%s): %v", xmlFile, err)
	}

	opts := MultiManifestFSOptions{
		ManifestDir: mfDir,
	}
	fs := NewMultiManifestFS(fix.service, fix.cache, opts)

	if err := fix.mount(fs); err != nil {
		t.Fatalf("mount: %v", err)
	}

	wsDir := filepath.Join(fix.mntDir, "ws")
	if _, err := os.Lstat(wsDir); err != nil {
		t.Fatalf("Lstat(%s): %v", wsDir, err)
	}

	if err := os.Remove(filepath.Join(fix.mntDir, "config", "ws")); err != nil {
		t.Fatalf("Remove(config link): %v", err)
	}

	if fi, err := os.Lstat(filepath.Join(mfDir, "ws")); err == nil {
		t.Errorf("'ws' still in manifest dir: %v", fi)
	}

	f, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("TempFile: %v", err)
	}
	if err := ioutil.WriteFile(f.Name(), []byte(testManifestXML), 0644); err != nil {
		t.Errorf("WriteFile(%s): %v", xmlFile, err)
	}

	configName := filepath.Join(fix.mntDir, "config", "ws2")
	if err := os.Symlink(f.Name(), configName); err != nil {
		t.Fatalf("Symlink(%s):  %v", xmlFile, err)
	}

	// XML file appears again.
	xmlFile = filepath.Join(mfDir, "ws2")
	if _, err := os.Stat(xmlFile); err != nil {
		t.Errorf("Stat(%s): %v", xmlFile, err)
	}
}
