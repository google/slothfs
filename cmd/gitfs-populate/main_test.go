package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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
	parent := newRepoTree(dir)
	construct(dir, "", parent)

	songT := &repoTree{
		children: map[string]*repoTree{},
		entries:  []string{"song.mp3"},
	}
	coreT := &repoTree{
		children: map[string]*repoTree{"song": songT},
		entries: []string{"subdir/core.h",
			"top",
		},
	}
	topT := &repoTree{
		children: map[string]*repoTree{
			"build/core": coreT,
		},
		entries: []string{
			"build/subfile",
			"toplevel",
		},
	}

	if !reflect.DeepEqual(topT, parent) {
		t.Errorf("got %#v want %#v", parent, topT)
	}

	all := topT.allChildren()
	want := map[string]*repoTree{
		"":                topT,
		"build/core":      coreT,
		"build/core/song": songT,
	}

	if !reflect.DeepEqual(all, want) {
		t.Errorf("got %#v want %#v", all, want)
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

}
