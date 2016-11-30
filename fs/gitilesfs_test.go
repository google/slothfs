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
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/manifest"
)

const fuseDebug = false

const testEncodedBlob = `IyBUaGlzIGlzIHRoZSBvZmZpY2lhbCBsaXN0IG9mIGdsb2cgYXV0aG9ycyBmb3IgY29weXJpZ2h0IHB1cnBvc2VzLgojIFRoaXMgZmlsZSBpcyBkaXN0aW5jdCBmcm9tIHRoZSBDT05UUklCVVRPUlMgZmlsZXMuCiMgU2VlIHRoZSBsYXR0ZXIgZm9yIGFuIGV4cGxhbmF0aW9uLgojCiMgTmFtZXMgc2hvdWxkIGJlIGFkZGVkIHRvIHRoaXMgZmlsZSBhczoKIwlOYW1lIG9yIE9yZ2FuaXphdGlvbiA8ZW1haWwgYWRkcmVzcz4KIyBUaGUgZW1haWwgYWRkcmVzcyBpcyBub3QgcmVxdWlyZWQgZm9yIG9yZ2FuaXphdGlvbnMuCiMKIyBQbGVhc2Uga2VlcCB0aGUgbGlzdCBzb3J0ZWQuCgpLb3VoZWkgU3V0b3UgPGtvdUBjb3ptaXhuZy5vcmc+Ckdvb2dsZSBJbmMuCg==`

var testBlob []byte

func init() {
	enc := map[string]string{
		"/platform/build/kati/+show/ce34badf691d36e8048b63f89d1a86ee5fa4325c/AUTHORS?format=TEXT":  testEncodedBlob,
		"/platform/build/kati/+show/ce34badf691d36e8048b63f89d1a86ee5fa4325c/AUTHORSx?format=TEXT": testEncodedBlob,
		"/platform/build/kati/+show/ce34badf691d36e8048b63f89d1a86ee5fa4325c/AUTHORS2?format=TEXT": testEncodedBlob,
		"/platform/build/kati/+/ce34badf691d36e8048b63f89d1a86ee5fa4325c/testcase/addprefix.mk":    "dGVzdDoKCWVjaG8gJChhZGRwcmVmaXggc3JjLyxmb28gYmFyKQo=",
	}
	for k, v := range enc {
		c := make([]byte, base64.StdEncoding.DecodedLen(len(v)))
		n, err := base64.StdEncoding.Decode(c, []byte(v))
		if err != nil {
			log.Panicf("Decode: %v", err)
		}

		c = c[:n]
		if v == testEncodedBlob {
			testBlob = c
		}

		testGitiles[k] = string(c)
	}
}

const testManifestXML = `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
  <remote  name="aosp"
           fetch=".."
           review="https://android-review.googlesource.com/" />
  <default revision="master"
           remote="aosp"
           sync-j="4" />
  <project path="build/kati" name="platform/build/kati" groups="pdk,tradefed" revision="ce34badf691d36e8048b63f89d1a86ee5fa4325c"
            clone-url="http://localhost/platform/platform/build/kati" >
    <copyfile dest="build/copydest" src="AUTHORS" />
    <linkfile dest="build/linkdest" src="AUTHORS" />
  </project>
</manifest>`

var testManifest *manifest.Manifest

func init() {
	var err error
	testManifest, err = manifest.Parse([]byte(testManifestXML))
	if err != nil {
		log.Panicf("manifest.Parse: %v", err)
	}
}

var testGitiles = map[string]string{
	"/platform/manifest/+show/master/default.xml?format=TEXT": testManifestXML,
	"/?format=JSON": `)]}'
{
  "platform/build/kati": {
    "name": "platform/build/kati",
    "clone_url": "https://android.googlesource.com/platform/build/kati",
    "description": "Description."
  }
}
`,
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
      "id": "787d767f94fd634ed29cd69ec9f93bab2b25f5d4",
      "name": "AUTHORS2",
      "size": 373
    },
    {
      "mode": 33261,
      "type": "blob",
      "id": "787d767f94fd634ed29cd69ec9f93bab2b25f5d4",
      "name": "AUTHORSx",
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

type testServer struct {
	addr     string
	listener net.Listener
	mu       sync.Mutex
	requests map[string]int
}

func (s *testServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	log.Println("handling", r.URL.String())

	s.mu.Lock()
	s.requests[r.URL.Path]++
	s.mu.Unlock()

	resp, ok := testGitiles[r.URL.String()]
	if !ok {
		http.Error(w, "not found", 404)
		return
	}

	out := []byte(resp)

	if strings.Contains(r.URL.String(), "format=TEXT") {
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		str := base64.StdEncoding.EncodeToString(out)
		w.Write([]byte(str))
	} else {
		w.Write([]byte(resp))
	}
}

func newTestServer() (*testServer, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}

	addr := listener.Addr().(*net.TCPAddr)

	ts := &testServer{
		listener: listener,
		requests: map[string]int{},
		addr:     fmt.Sprintf("localhost:%d", addr.Port),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", ts.handleStatic)

	s := &http.Server{
		Handler: mux,
	}

	go s.Serve(ts.listener)

	return ts, err
}

func TestGitilesFSNotInGit(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	// Add a git repo; this doesn't have the requested blob, but
	// we can still get it from our (fake) HTTP gitiles server.
	cmd := exec.Command("/bin/sh", "-c",
		strings.Join([]string{
			"mkdir -p localhost/platform/build/kati.git",
			"cd localhost/platform/build/kati.git",
			"git init",
			"touch file",
			"git add file",
			"git commit -m msg -a"}, " && "))
	cmd.Dir = filepath.Join(fix.dir, "cache", "git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create repo: %v, out: %s", err, string(out))
	}

	repoService := fix.service.NewRepoService("platform/build/kati")
	treeResp, err := repoService.GetTree("ce34badf691d36e8048b63f89d1a86ee5fa4325c", "", true)
	if err != nil {
		t.Fatal("Tree:", err)
	}

	options := GitilesRevisionOptions{
		Revision: "ce34badf691d36e8048b63f89d1a86ee5fa4325c",
		GitilesOptions: GitilesOptions{
			CloneURL: fmt.Sprintf("http://%s/platform/build/kati", fix.testServer.addr),
		},
	}

	fs := NewGitilesRoot(fix.cache, treeResp, repoService, options)
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	if _, err := ioutil.ReadFile(filepath.Join(fix.mntDir, "AUTHORS")); err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
}

func TestGitilesFSSharedNodes(t *testing.T) {
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

	options := GitilesRevisionOptions{}

	fs := NewGitilesRoot(fix.cache, treeResp, repoService, options)
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	ch1 := fs.Inode().GetChild("AUTHORS")
	if ch1 == nil {
		t.Fatalf("node for AUTHORS not found")
	}

	ch2 := fs.Inode().GetChild("AUTHORS2")
	if ch2 == nil {
		t.Fatalf("node for AUTHORS2 not found")
	}

	if ch1 != ch2 {
		t.Error("equal blobs did not share inodes.")
	}
	ch3 := fs.Inode().GetChild("AUTHORSx")
	if ch1 == ch3 {
		t.Error("blob with different modes shared inode.")
	}
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

	options := GitilesRevisionOptions{}

	fs := NewGitilesRoot(fix.cache, treeResp, repoService, options)
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	want := "58d9fdae2c26d82e04f3fcafc4358b99109f0e70"
	path := filepath.Join(fix.mntDir, ".slothfs/treeID")
	if got, err := ioutil.ReadFile(path); err != nil {
		t.Errorf("ReadFile(.slothfs/treeID): %v", err)
	} else if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}

	data := make([]byte, 1024)
	sz, err := syscall.Listxattr(filepath.Join(fix.mntDir, "AUTHORS"), data)
	if err != nil {
		t.Fatalf("Listxattr: %v", err)
	}
	if got, want := string(data[:sz]), xattrName+"\000"; got != want {
		t.Errorf("got xattrs %q, want %q", got, want)
	}

	sz, err = syscall.Getxattr(filepath.Join(fix.mntDir, "AUTHORS"), xattrName, data)
	if err != nil {
		t.Fatalf("Getxattr: %v", err)
	}
	if got, want := "787d767f94fd634ed29cd69ec9f93bab2b25f5d4", string(data[:sz]); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGitilesFSSubmodule(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	repoService := fix.service.NewRepoService("platform/build/kati")

	tree := &gitiles.Tree{
		ID: "ffffbadf691d36e8048b63f89d1a86ee5fa4325c",
		Entries: []gitiles.TreeEntry{{
			Name: "submod",
			Type: "commit",
			ID:   "ce34badf691d36e8048b63f89d1a86ee5fa4325c",
		}},
	}
	fs := NewGitilesRoot(fix.cache, tree, repoService, GitilesRevisionOptions{})
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	if fi, err := os.Lstat(filepath.Join(fix.mntDir, "submod")); err != nil {
		t.Fatalf("Stat(submod): %v", err)
	} else if !fi.IsDir() {
		t.Errorf("Stat(submod): got mode %x, want dir", fi.Mode())
	}
}

func TestGitilesFSBasic(t *testing.T) {
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

	options := GitilesRevisionOptions{}
	options.CloneOption = fileOpts

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

func TestGitilesFSCachedRead(t *testing.T) {
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

	options := GitilesRevisionOptions{
		Revision: "ce34badf691d36e8048b63f89d1a86ee5fa4325c",
	}

	fs := NewGitilesRoot(fix.cache, treeResp, repoService, options)
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := ioutil.ReadFile(filepath.Join(fix.mntDir, "AUTHORS")); err != nil {
			t.Fatalf("ReadFile %d: %v", i, err)
		}
	}

	ch := fs.Inode().GetChild("AUTHORS")
	if ch == nil {
		t.Fatalf("node for AUTHORS not found")
	}

	giNode, ok := ch.Node().(*gitilesNode)
	if !ok {
		t.Fatalf("got node type %T, want *gitilesNode", ch.Node())
	}

	if c := atomic.LoadUint32(&giNode.readCount); c != 1 {
		t.Errorf("inode was read %d times, want 1.", c)
	}
}

func TestGitilesFSTimeStamps(t *testing.T) {
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

	fs := NewGitilesRoot(fix.cache, treeResp, repoService, GitilesRevisionOptions{})
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	fn := filepath.Join(fix.mntDir, "testcase", "addprefix.mk")

	n := time.Now()
	if err := os.Chtimes(fn, n, n); err != nil {
		t.Errorf("Chtimes: %v", err)
	}

	after, err := os.Lstat(fn)
	if err != nil {
		t.Fatalf("Lstat(%q): %v", fn, err)
	}

	if !n.Equal(after.ModTime()) {
		t.Errorf("mod time did not change")
	}
}

func TestGitilesFSMultiFetch(t *testing.T) {
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

	options := GitilesRevisionOptions{
		Revision: "ce34badf691d36e8048b63f89d1a86ee5fa4325c",
	}

	fs := NewGitilesRoot(fix.cache, treeResp, repoService, options)
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	fn := filepath.Join(fix.mntDir, "AUTHORS")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			ioutil.ReadFile(fn)
			wg.Done()
		}()
	}
	wg.Wait()

	for key, got := range fix.testServer.requests {
		if got != 1 {
			t.Errorf("got request count %d for %s, want 1", got, key)
		}
	}
}

func TestGitilesConfigFSTest(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	repoService := fix.service.NewRepoService("platform/build/kati")
	if err != nil {
		t.Fatal("Tree:", err)
	}

	fs := NewGitilesConfigFSRoot(fix.cache, repoService, &GitilesOptions{})
	if err := fix.mount(fs); err != nil {
		t.Fatal("mount", err)
	}

	fn := filepath.Join(fix.mntDir, "ce34badf691d36e8048b63f89d1a86ee5fa4325c", "AUTHORS")
	content, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Compare(content, testBlob) != 0 {
		t.Errorf("blob for %s differs", fn)
	}
}

func TestGitilesHostFS(t *testing.T) {
	fix, err := newTestFixture()
	if err != nil {
		t.Fatal("newTestFixture", err)
	}
	defer fix.cleanup()

	if fs, err := NewHostFS(fix.cache, fix.service, nil); err != nil {
		t.Fatalf("NewHostFS: %v", err)
	} else if err := fix.mount(fs); err != nil {
		t.Fatalf("mount: %v", err)
	}

	fn := filepath.Join(fix.mntDir, "platform/build/kati", "ce34badf691d36e8048b63f89d1a86ee5fa4325c", "AUTHORS")
	content, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Compare(content, testBlob) != 0 {
		t.Errorf("blob for %s differs", fn)
	}
}
