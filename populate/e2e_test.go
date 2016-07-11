package populate

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/fs"
	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/manifest"
	"github.com/hanwen/go-fuse/fuse/nodefs"

	git "github.com/libgit2/git2go"
)

// a bunch of random sha1s.
var ids = []string{
	"f065f1478dc8bfebdc59f20fb2fc1f8da4d7c334",
	"ae6d11c113a0a20be662df287899046f74092abe",
	"9200e4a97b6e051dd56d3de5378febae40a367e9",
	"7ba00d0407ed4467c874ab45bb47fcb82fe63fac",
}

func gitID(s string) *git.Oid {
	i, err := git.NewOid(s)
	if err != nil {
		log.Panicf("NewOid(%q): %v", i, err)
	}
	return i
}

func newInt(i int) *int {
	return &i
}

func abortListener(l net.Listener) {
	_, err := l.Accept()
	if err == nil {
		log.Panicf("got incoming connection")
	}
}

func TestFUSE(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, d := range []string{"mnt", "ws", "cache"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	cache, err := cache.NewCache(filepath.Join(dir, "cache"), cache.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Setup a fake gitiles; make sure we never talk to it.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go abortListener(l)
	defer l.Close()

	service, err := gitiles.NewService(fmt.Sprintf("http://%s/", l.Addr()), gitiles.Options{})
	if err != nil {
		log.Printf("NewService: %v", err)
	}

	opts := fs.MultiFSOptions{}

	root := fs.NewMultiFS(service, cache, opts)
	fuseOpts := nodefs.NewOptions()
	server, _, err := nodefs.MountRoot(filepath.Join(dir, "mnt"), root, fuseOpts)
	if err != nil {
		t.Fatal(err)
	}

	go server.Serve()
	defer server.Unmount()

	// We avoid talking to gitiles by inserting entries into the
	// cache manually.
	if err := cache.Tree.Add(gitID(ids[0]), &gitiles.Tree{
		ID: ids[0],
		Entries: []gitiles.TreeEntry{
			{
				Mode: 0100644,
				Name: "a",
				Type: "blob",
				ID:   ids[1],
				Size: newInt(42),
			},
			{
				Mode: 0100644,
				Name: "b/c",
				Type: "blob",
				ID:   ids[2],
				Size: newInt(1),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := cache.Tree.Add(gitID(ids[1]), &gitiles.Tree{
		ID: ids[1],
		Entries: []gitiles.TreeEntry{
			{
				Mode: 0100644,
				Name: "a",
				Type: "blob",
				ID:   ids[2],
				Size: newInt(42),
			},
			{
				Mode: 0100644,
				Name: "b/c",
				Type: "blob",
				ID:   ids[2],
				Size: newInt(1),
			},
			{
				Mode: 0100644,
				Name: "new",
				Type: "blob",
				ID:   ids[3],
				Size: newInt(42),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := cache.Tree.Add(gitID(ids[2]), &gitiles.Tree{
		ID: ids[2],
		Entries: []gitiles.TreeEntry{
			{
				Mode: 0100644,
				Name: "d",
				Type: "blob",
				ID:   ids[3],
				Size: newInt(42),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	mf1 := manifest.Manifest{
		Project: []manifest.Project{{
			Name:     "platform/project",
			Path:     "project",
			Revision: ids[0],
		}}}

	mf2 := manifest.Manifest{
		Project: []manifest.Project{
			{
				Name:     "platform/project",
				Path:     "project",
				Revision: ids[1],
			}, {
				Name:     "platform/sub",
				Path:     "sub",
				Revision: ids[2],
			}},
	}

	bytes1, err := mf1.MarshalXML()
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "m1.xml"), bytes1, 0644); err != nil {
		t.Fatal(err)
	}

	bytes2, err := mf2.MarshalXML()
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "m2.xml"), bytes2, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(filepath.Join(dir, "m1.xml"), filepath.Join(dir, "mnt", "config", "m1")); err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(dir, "mnt", "m1", "project", "b/c")
	if fi, err := os.Lstat(testFile); err != nil {
		t.Fatalf("Lstat(%s): %v", testFile, err)
	} else if fi.Size() != 1 {
		t.Fatalf("%s has size %d", fi.Size())
	}

	if err := os.Symlink(filepath.Join(dir, "m2.xml"), filepath.Join(dir, "mnt", "config", "m2")); err != nil {
		t.Fatal(err)
	}

	ws := filepath.Join(dir, "ws")

	if _, err := Checkout(filepath.Join(dir, "mnt", "m1"), ws); err != nil {
		t.Fatal("Checkout m1:", err)
	}

	if dest, err := os.Readlink(filepath.Join(ws, "project")); err != nil {
		t.Fatal(err)
	} else if want := filepath.Join(dir, "mnt", "m1", "project"); dest != want {
		t.Fatalf("got %q, want %q", dest, want)
	}

	// Make sure we detect changed files. We have to be careful in
	// the test setup that no blobs are shared with newly
	// appearing files, or they'll be touched for being new files.

	changed, err := Checkout(filepath.Join(dir, "mnt", "m2"), ws)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{"project/a", "project/new"} {
		found := false
		for _, c := range changed {
			if c == filepath.Join(dir, "mnt", "m2", f) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("file %s was not changed.", f)
		}
	}

	if dest, err := os.Readlink(filepath.Join(ws, "sub")); err != nil {
		t.Fatal(err)
	} else if want := filepath.Join(dir, "mnt", "m2", "sub"); dest != want {
		t.Fatalf("got %q, want %q", dest, want)
	}
}
