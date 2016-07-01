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

package cache

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/google/slothfs/gitiles"
	git "github.com/libgit2/git2go"
)

func newInt(i int) *int          { return &i }
func newString(s string) *string { return &s }

func TestGetTree(t *testing.T) {
	testRepo, err := initTest()
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer testRepo.Cleanup()

	treeResp, err := GetTree(testRepo.repo, testRepo.treeID)
	if err != nil {
		t.Fatalf("getTree: %v", err)
	}

	str := "abcd1234abcd1234abcd1234abcd1234abcd1234"

	want := []gitiles.TreeEntry{
		{
			ID:   str,
			Name: "dir/f1",
			Type: "blob",
			Mode: 0100644,
			Size: newInt(5),
		},
		{
			Name: "dir/f2",
			Type: "blob",
			Mode: 0100755,
			ID:   str,
			Size: newInt(11),
		},
		{
			ID:     str,
			Name:   "link",
			Type:   "blob",
			Mode:   0120000,
			Size:   newInt(5),
			Target: newString("hello"),
		},
	}
	if len(treeResp.Entries) != 3 {
		t.Fatalf("got %d entries, want 3 entries", len(treeResp.Entries))
	}
	for i := range treeResp.Entries {
		treeResp.Entries[i].ID = str
		if !reflect.DeepEqual(want[i], treeResp.Entries[i]) {
			t.Errorf("entry %d: got %v, want %v", i, &treeResp.Entries[i], &want[i])
		}
	}
}

func TestTreeCache(t *testing.T) {
	testRepo, err := initTest()
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer testRepo.Cleanup()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}

	cache := &TreeCache{dir}

	treeResp, err := GetTree(testRepo.repo, testRepo.treeID)
	if err != nil {
		t.Fatalf("getTree: %v", err)
	}

	randomID, err := git.NewOid("abcd1234abcd1234abcd1234abcd1234abcd1234")
	if err != nil {
		t.Fatalf("NewOid: %v", err)
	}

	if err := cache.Add(randomID, treeResp); err != nil {
		t.Fatalf("cache.add %v", err)
	}

	roundtrip, err := cache.Get(randomID)
	if err != nil {
		t.Fatalf("cache.get: %v", err)
	}
	if !reflect.DeepEqual(roundtrip, treeResp) {
		t.Fatalf("got %#v, want %#v", roundtrip, treeResp)
	}

	asTree, err := cache.Get(testRepo.treeID)
	if err != nil {
		t.Fatalf("cache.get: %v", err)
	}
	if !reflect.DeepEqual(asTree, treeResp) {
		t.Fatalf("got %#v, want %#v", roundtrip, treeResp)
	}
}

type testRepo struct {
	dir       string
	subTreeID *git.Oid
	treeID    *git.Oid
	repo      *git.Repository
}

func (r *testRepo) Cleanup() {
	os.RemoveAll(r.dir)
}

func initTest() (*testRepo, error) {
	d, err := ioutil.TempDir("", "tmpgit")
	if err != nil {
		return nil, err
	}

	repo, err := git.InitRepository(d, true)
	if err != nil {
		return nil, err
	}

	c1 := []byte("hello")
	c2 := []byte("goedemiddag")

	id1, err := repo.CreateBlobFromBuffer(c1)
	if err != nil {
		return nil, err
	}

	id2, err := repo.CreateBlobFromBuffer(c2)
	if err != nil {
		return nil, err
	}
	b, err := repo.TreeBuilder()

	if err != nil {
		return nil, err
	}
	defer b.Free()

	if err = b.Insert("f1", id1, git.FilemodeBlob); err != nil {
		return nil, err
	}

	if err := b.Insert("f2", id2, git.FilemodeBlobExecutable); err != nil {
		return nil, err
	}
	subTreeID, err := b.Write()

	b, err = repo.TreeBuilder()
	if err != nil {
		return nil, err
	}
	defer b.Free()

	if err = b.Insert("dir", subTreeID, git.FilemodeTree); err != nil {
		return nil, err
	}
	if err = b.Insert("link", id1, git.FilemodeLink); err != nil {
		return nil, err
	}
	treeID, err := b.Write()
	if err != nil {
		return nil, err
	}

	return &testRepo{
		dir:       d,
		repo:      repo,
		treeID:    treeID,
		subTreeID: subTreeID,
	}, nil
}
