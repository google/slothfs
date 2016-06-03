package cache

import (
	"io/ioutil"
	"testing"
	"time"
)

func TestGitCache(t *testing.T) {
	testRepo, err := initTest()
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer testRepo.Cleanup()

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}

	cache, err := newGitCache(dir)
	if err != nil {
		t.Fatalf("newGitCache(%s): %v", dir, err)
	}

	url := "file://" + testRepo.dir

	lazy := newLazyRepo(url, cache)
	if r := lazy.Repository(); r != nil {
		t.Errorf("got %v for lazy.Repository", r)
	}

	go lazy.Clone()
	if r := lazy.Repository(); r != nil {
		t.Errorf("got %v for lazy.Repository", r)
	}

	// The API doesn't let us synchronize on finished clone, so we
	// have no better way to test than sleep. This test may be
	// flaky on highly loaded machines.
	dt := 50 * time.Millisecond
	time.Sleep(dt)

	if repo := lazy.Repository(); repo == nil {
		t.Errorf("lazyRepo still not loaded after %s.", dt)
	}
}
