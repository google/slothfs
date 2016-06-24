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
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"github.com/google/gitfs/cache"
	"github.com/google/gitfs/gitiles"
	"github.com/google/gitfs/manifest"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

const fuseDebug = false

func init() {
	enc := map[string]string{
		"/platform/build/kati/+show/ce34badf691d36e8048b63f89d1a86ee5fa4325c/AUTHORS?format=TEXT": `IyBUaGlzIGlzIHRoZSBvZmZpY2lhbCBsaXN0IG9mIGdsb2cgYXV0aG9ycyBmb3IgY29weXJpZ2h0IHB1cnBvc2VzLgojIFRoaXMgZmlsZSBpcyBkaXN0aW5jdCBmcm9tIHRoZSBDT05UUklCVVRPUlMgZmlsZXMuCiMgU2VlIHRoZSBsYXR0ZXIgZm9yIGFuIGV4cGxhbmF0aW9uLgojCiMgTmFtZXMgc2hvdWxkIGJlIGFkZGVkIHRvIHRoaXMgZmlsZSBhczoKIwlOYW1lIG9yIE9yZ2FuaXphdGlvbiA8ZW1haWwgYWRkcmVzcz4KIyBUaGUgZW1haWwgYWRkcmVzcyBpcyBub3QgcmVxdWlyZWQgZm9yIG9yZ2FuaXphdGlvbnMuCiMKIyBQbGVhc2Uga2VlcCB0aGUgbGlzdCBzb3J0ZWQuCgpLb3VoZWkgU3V0b3UgPGtvdUBjb3ptaXhuZy5vcmc+Ckdvb2dsZSBJbmMuCg==`,
		"/platform/build/kati/+/ce34badf691d36e8048b63f89d1a86ee5fa4325c/testcase/addprefix.mk":   "dGVzdDoKCWVjaG8gJChhZGRwcmVmaXggc3JjLyxmb28gYmFyKQo=",
	}
	for k, v := range enc {
		c := make([]byte, base64.StdEncoding.DecodedLen(len(v)))
		n, err := base64.StdEncoding.Decode(c, []byte(v))
		if err != nil {
			log.Panicf("Decode: %v", err)
		}

		c = c[:n]
		testGitiles[k] = string(c)
	}
}

const testManifest = `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
  <remote  name="aosp"
           fetch=".."
           review="https://android-review.googlesource.com/" />
  <default revision="master"
           remote="aosp"
           sync-j="4" />
  <project path="build/kati" name="platform/build/kati" groups="pdk,tradefed" revision="ce34badf691d36e8048b63f89d1a86ee5fa4325c">
    <copyfile dest="build/copydest" src="AUTHORS" />
    <linkfile dest="build/linkdest" src="AUTHORS" />
  </project>
</manifest>`

var testGitiles = map[string]string{
	"/platform/manifest/+show/master/default.xml?format=TEXT": testManifest,
	"/platform/build/kati/+/master?format=JSON": `)]}'
{
  "commit": "ce34badf691d36e8048b63f89d1a86ee5fa4325c",
  "tree": "58d9fdae2c26d82e04f3fcafc4358b99109f0e70",
  "parents": [
    "c2c5246e3ad95e1c0fa81a1f8344916ff68588bf",
    "becba507595aaf6940af662c9096dbabe50baba4"
  ],
  "author": {
    "name": "Shinichiro Hamaji",
    "email": "hamaji@google.com",
    "time": "Tue Apr 12 15:29:01 2016 +0900"
  },
  "committer": {
    "name": "Shinichiro Hamaji",
    "email": "hamaji@google.com",
    "time": "Tue Apr 12 15:29:17 2016 +0900"
  },
  "message": "Merge remote-tracking branch \u0027aosp/upstream\u0027\n\nTwo bug fixes. becba50 is actually for a long lived bug, but\nwas found by recent endif/endef checks. Without 706c27f, you\ncannot debug ckati binary on Mac.\n\nbecba50 [C++] Strip a trailing \\r\n706c27f Handle EINTR on read\n\nBug: 28087626\nChange-Id: Ic0c24873a49be96afc83078b6a41960bce444d57\n",
  "tree_diff": []
}`,
	"/platform/build/kati/+/ce34badf691d36e8048b63f89d1a86ee5fa4325c/?format=JSON&long=1&recursive=1": `)]}'
{
  "id": "58d9fdae2c26d82e04f3fcafc4358b99109f0e70",
  "entries": [
    {
      "mode": 33188,
      "type": "blob",
      "id": "787d767f94fd634ed29cd69ec9f93bab2b25f5d4",
      "name": "AUTHORS",
      "size": 373
    },
    {
      "mode": 33188,
      "type": "blob",
      "id": "91c29720b08211898308eb2b6bde8bd3208c6dcd",
      "name": "Android.bp",
      "size": 1935
    },
    {
      "mode": 33188,
      "type": "blob",
      "id": "bdea84459e8c5266251248e593c8ba226a535ad2",
      "name": "testcase/addprefix.mk",
      "size": 38
    },
    {
      "mode": 33188,
      "type": "blob",
      "id": "072b5fc6ca14a64f35f7841080e4b9c972c89b3d",
      "name": "testcase/addsuffix.mk",
      "size": 36
    }
  ]
}
`,
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	log.Println("handling", r.URL.String())
	resp, ok := testGitiles[r.URL.String()]
	if !ok {
		http.Error(w, "not found", 404)
	}

	out := []byte(resp)

	if strings.Contains(r.URL.String(), "format=TEXT") {
		str := base64.StdEncoding.EncodeToString(out)
		w.Write([]byte(str))
	} else {
		w.Write([]byte(resp))
	}
	// TODO(hanwen): set content type?
}

func newTestServer() (net.Listener, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleStatic)

	s := &http.Server{
		Handler: mux,
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	go s.Serve(listener)
	return listener, err
}

func TestGitilesFSTreeID(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	repoService := fix.service.NewRepoService("platform/build/kati")
	treeResp, err := repoService.GetTree("ce34badf691d36e8048b63f89d1a86ee5fa4325c", "", true)
	if err != nil {
		t.Fatal("Tree:", err)
	}

	options := GitilesOptions{}

	fs := NewGitilesRoot(fix.cache, treeResp, repoService, options)
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	want := "58d9fdae2c26d82e04f3fcafc4358b99109f0e70"
	path := filepath.Join(fix.mntDir, ".gitid")
	if got, err := ioutil.ReadFile(path); err != nil {
		t.Errorf("ReadFile(.gitid): %v", err)
	} else if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGitilesFS(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	fileOpts := []CloneOption{
		{
			RE:    regexp.MustCompile(".*\\.mk$"),
			Clone: false,
		}, {
			RE:    regexp.MustCompile(".*"),
			Clone: true,
		}}

	repoService := fix.service.NewRepoService("platform/build/kati")
	treeResp, err := repoService.GetTree("ce34badf691d36e8048b63f89d1a86ee5fa4325c", "", true)
	if err != nil {
		t.Fatal("Tree:", err)
	}

	options := GitilesOptions{
		CloneOption: fileOpts,
	}

	fs := NewGitilesRoot(fix.cache, treeResp, repoService, options)
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	fn := filepath.Join(fix.mntDir, "testcase", "addprefix.mk")
	if fi, err := os.Lstat(fn); err != nil {
		t.Fatalf("Lstat(%q): %v", fn, err)
	} else {
		if fi.Size() != 38 {
			t.Errorf("Lstat(%q): got size %d want 38", fn, fi.Size())
		}
	}

	// TODO(hanwen): is this race-detector sane?
	ch := fs.Inode().GetChild("testcase")
	if ch == nil {
		t.Fatalf("node for testcase/ not found")
	}
	ch = ch.GetChild("addprefix.mk")
	if ch == nil {
		t.Fatalf("node for addprefix.mk not found")
	}

	giNode, ok := ch.Node().(*gitilesNode)
	if !ok {
		t.Fatalf("got node type %T, want *gitilesNode", ch.Node())
	}

	if giNode.clone {
		t.Errorf(".mk file had clone set.")
	}
}

func TestManifestFSCloneOption(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	mf, err := manifest.Parse([]byte(testManifest))
	if err != nil {
		t.Fatal("manifest.Parse:", err)
	}

	for i := range mf.Project {
		mf.Project[i].CloneDepth = "1"
	}

	opts := ManifestOptions{
		Manifest: mf,
	}

	fs, err := NewManifestFS(fix.service, fix.cache, opts)
	if err != nil {
		t.Fatalf("NewManifestFS: %v", err)
	}
	if err := fix.mount(fs); err != nil {
		log.Fatalf("MountFileSystem: %v", err)
	}

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

func TestManifestFS(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	mf, err := manifest.Parse([]byte(testManifest))
	if err != nil {
		t.Fatal("manifest.Parse:", err)
	}

	opts := ManifestOptions{
		Manifest: mf,
	}

	fs, err := NewManifestFS(fix.service, fix.cache, opts)
	if err != nil {
		t.Fatalf("NewManifestFS: %v", err)
	}
	if err := fix.mount(fs); err != nil {
		log.Fatalf("MountFileSystem: %v", err)
	}

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
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	mf, err := manifest.Parse([]byte(testManifest))
	if err != nil {
		t.Fatal("manifest.Parse:", err)
	}

	opts := ManifestOptions{
		Manifest: mf,
	}

	fs, err := NewManifestFS(fix.service, fix.cache, opts)
	if err != nil {
		t.Fatalf("NewManifestFS: %v", err)
	}
	if err := fix.mount(fs); err != nil {
		t.Fatalf("mount: %v", err)
	}

	xmlPath := filepath.Join(fix.mntDir, "manifest.xml")
	fuseMF, err := manifest.ParseFile(xmlPath)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", xmlPath, err)
	}

	if !reflect.DeepEqual(fuseMF, mf) {
		t.Errorf("read back manifest %v, want %v", fuseMF, mf)
	}
}

type testFixture struct {
	dir      string
	mntDir   string
	server   *fuse.Server
	cache    *cache.Cache
	listener net.Listener
	service  *gitiles.Service
}

func (f *testFixture) cleanup() {
	if f.listener != nil {
		f.listener.Close()
	}
	if f.server != nil {
		f.server.Unmount()
	}
	os.RemoveAll(f.dir)
}

func newTestFixture() (*testFixture, error) {
	d, err := ioutil.TempDir("", "multifstest")
	if err != nil {
		return nil, err
	}

	fixture := &testFixture{dir: d}

	fixture.cache, err = cache.NewCache(filepath.Join(d, "/cache"))
	if err != nil {
		return nil, err
	}

	fixture.listener, err = newTestServer()
	if err != nil {
		return nil, err
	}

	fixture.service, err = gitiles.NewService(
		fmt.Sprintf("http://%s", fixture.listener.Addr().String()))
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

	var err error
	f.server, _, err = nodefs.MountRoot(f.mntDir, root, nil)
	if err != nil {
		return err
	}

	if fuseDebug {
		f.server.SetDebug(true)
	}
	go f.server.Serve()
	return nil
}

func TestMultiFS(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatalf("newTestFixture: %v", err)
	}
	defer fix.cleanup()

	xmlFile := filepath.Join(fix.dir, "manifest.xml")
	if err := ioutil.WriteFile(xmlFile, []byte(testManifest), 0644); err != nil {
		t.Errorf("WriteFile(%s): %v", xmlFile, err)
	}

	brokenXMLFile := filepath.Join(fix.dir, "broken.xml")
	if err := ioutil.WriteFile(brokenXMLFile, []byte("I'm not XML."), 0644); err != nil {
		t.Errorf("WriteFile(%s): %v", brokenXMLFile, err)
	}

	opts := MultiFSOptions{}
	fs := NewMultiFS(fix.service, fix.cache, opts)

	if err := fix.mount(fs); err != nil {
		t.Fatalf("mount: %v", err)
	}

	if err := os.Symlink(brokenXMLFile, filepath.Join(fix.mntDir, "config", "ws")); err == nil {
		t.Fatalf("want error for broken XML file")
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
		t.Fatalf("Readlink(%s): %", configName, err)
	} else if want := "../ws/manifest.xml"; got != want {
		t.Errorf("got link %s, want %s", got, want)
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
