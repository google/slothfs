package populate

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/fs"
	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/manifest"
	"github.com/hanwen/go-fuse/fuse"
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

func newString(s string) *string {
	return &s
}

func abortListener(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			break
		}
		conn.Close()
	}
}

type fixture struct {
	dir          string
	cache        *cache.Cache
	fsServer     *fuse.Server
	abortGitiles net.Listener
}

func (f *fixture) Cleanup() {
	if f.abortGitiles != nil {
		f.abortGitiles.Close()
	}
	if f.fsServer != nil {
		if err := f.fsServer.Unmount(); err != nil {
			return
		}
	}
	os.RemoveAll(f.dir)
}

func (f *fixture) addWorkspace(name string, mf *manifest.Manifest) error {
	bytes1, err := mf.MarshalXML()
	if err != nil {
		return err
	}

	dir := f.dir

	if err := ioutil.WriteFile(filepath.Join(dir, name+".xml"), bytes1, 0644); err != nil {
		return err
	}
	if err := os.Symlink(filepath.Join(dir, name+".xml"), filepath.Join(dir, "mnt", "config", name)); err != nil {
		return err
	}
	return nil
}

func newFixture() (*fixture, error) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}

	fix := fixture{dir: dir}
	for _, d := range []string{"mnt", "ws", "cache"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			return nil, err
		}
	}

	fix.cache, err = cache.NewCache(filepath.Join(dir, "cache"), cache.Options{})
	if err != nil {
		return nil, err
	}

	// Setup a fake gitiles; make sure we never talk to it.
	fix.abortGitiles, err = net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	go abortListener(fix.abortGitiles)

	service, err := gitiles.NewService(fmt.Sprintf("http://%s/", fix.abortGitiles.Addr()), gitiles.Options{})
	if err != nil {
		log.Printf("NewService: %v", err)
	}

	opts := fs.MultiFSOptions{}

	root := fs.NewMultiFS(service, fix.cache, opts)
	fuseOpts := nodefs.NewOptions()
	server, _, err := nodefs.MountRoot(filepath.Join(dir, "mnt"), root, fuseOpts)
	if err != nil {
		return nil, err
	}

	go server.Serve()

	return &fix, nil
}

func TestFUSESymlink(t *testing.T) {
	fixture, err := newFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Cleanup()

	// We avoid talking to gitiles by inserting entries into the
	// cache manually.
	if err := fixture.cache.Tree.Add(gitID(ids[0]), &gitiles.Tree{
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
				Mode:   0100644,
				Name:   "link",
				Type:   "blob",
				ID:     ids[2],
				Size:   newInt(1),
				Target: newString("non-existent"),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// We avoid talking to gitiles by inserting entries into the
	// cache manually.
	if err := fixture.cache.Tree.Add(gitID(ids[1]), &gitiles.Tree{
		ID: ids[1],
		Entries: []gitiles.TreeEntry{
			{
				Mode: 0100644,
				Name: "a",
				Type: "blob",
				ID:   ids[1],
				Size: newInt(42),
			},
			{
				Mode:   0100644,
				Name:   "link",
				Type:   "blob",
				ID:     ids[3],
				Size:   newInt(1),
				Target: newString("a"),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i <= 1; i++ {
		if err := fixture.addWorkspace(fmt.Sprintf("m%d", i), &manifest.Manifest{
			Project: []manifest.Project{{
				Name:     "platform/project",
				Path:     "p",
				Revision: ids[i],
			}}}); err != nil {
			t.Fatalf("addWorkspace(%d): %v", i, err)
		}
	}

	ws := filepath.Join(fixture.dir, "ws")
	m0 := filepath.Join(fixture.dir, "mnt", "m0")
	added, changed, err := Checkout(m0, ws)
	if err != nil {
		t.Fatalf("Checkout m0: %v", err)
	}
	if len(changed) > 0 {
		t.Errorf("got changed files %v on fresh checkout", changed)
	}
	if want := []string{filepath.Join(m0, "p/a"), filepath.Join(m0, "p/link")}; !reflect.DeepEqual(added, want) {
		t.Errorf("got added %v want %v on fresh checkout", added, want)
	}

	m1 := filepath.Join(fixture.dir, "mnt", "m1")
	added, changed, err = Checkout(m1, ws)
	if len(added) > 0 {
		t.Errorf("got added files %v on sync", added)
	}
	if want := []string{filepath.Join(m1, "p/link")}; !reflect.DeepEqual(changed, want) {
		t.Errorf("got changed files %v, want %v", changed, want)
	}
}

func TestBasic(t *testing.T) {
	fixture, err := newFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Cleanup()
	dir := fixture.dir

	// We avoid talking to gitiles by inserting entries into the
	// cache manually.
	if err := fixture.cache.Tree.Add(gitID(ids[0]), &gitiles.Tree{
		ID: ids[0],
		Entries: []gitiles.TreeEntry{
			{
				Mode: 0100644,
				Name: "a",
				Type: "blob",
				ID:   ids[1],
				Size: newInt(1),
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
	if err := fixture.cache.Tree.Add(gitID(ids[1]), &gitiles.Tree{
		ID: ids[1],
		Entries: []gitiles.TreeEntry{
			{
				Mode: 0100644,
				Name: "a",
				Type: "blob",
				ID:   ids[2],
				Size: newInt(1),
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
				Size: newInt(1),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := fixture.cache.Tree.Add(gitID(ids[2]), &gitiles.Tree{
		ID: ids[2],
		Entries: []gitiles.TreeEntry{
			{
				Mode: 0100644,
				Name: "d",
				Type: "blob",
				ID:   ids[3],
				Size: newInt(1),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := fixture.addWorkspace("m1", &manifest.Manifest{
		Project: []manifest.Project{{
			Name:     "platform/project",
			Path:     "project",
			Revision: ids[0],
		}}}); err != nil {
		t.Fatalf("addWorkspace(m1): %v", err)
	}

	if err := fixture.addWorkspace("m2", &manifest.Manifest{
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
	}); err != nil {
		t.Fatalf("addWorkspace(m2): %v", err)
	}

	testFile := filepath.Join(dir, "mnt", "m1", "project", "b/c")
	if fi, err := os.Lstat(testFile); err != nil {
		t.Fatalf("Lstat(%s): %v", testFile, err)
	} else if fi.Size() != 1 {
		t.Fatalf("%s has size %d", testFile, fi.Size())
	}

	ws := filepath.Join(dir, "ws")

	if _, _, err := Checkout(filepath.Join(dir, "mnt", "m1"), ws); err != nil {
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

	added, changed, err := Checkout(filepath.Join(dir, "mnt", "m2"), ws)
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{filepath.Join(dir, "mnt", "m2", "project/a")}; !reflect.DeepEqual(changed, want) {
		t.Errorf("got changed %v, want %v", changed, want)
	}

	if want := []string{
		filepath.Join(dir, "mnt", "m2", "project/new"),
		filepath.Join(dir, "mnt", "m2", "sub/d"),
	}; !reflect.DeepEqual(added, want) {
		t.Errorf("got added %v, want %v", added, want)
	}

	if dest, err := os.Readlink(filepath.Join(ws, "sub")); err != nil {
		t.Fatal(err)
	} else if want := filepath.Join(dir, "mnt", "m2", "sub"); dest != want {
		t.Fatalf("Readlink(ws/sub): got %q, want %q", dest, want)
	}
}
