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
	"os/exec"
	"strings"
	"testing"
	"time"

	git "gopkg.in/src-d/go-git.v4"
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
	defer os.RemoveAll(dir)

	cache, err := newGitCache(dir, Options{})
	if err != nil {
		t.Fatalf("newGitCache(%s): %v", dir, err)
	}

	url := "file://" + testRepo.dir
	if r := cache.OpenLocal(url); r != nil {
		t.Errorf("OpenLocal(%s) succeeded", url)
	}

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

	if r := cache.OpenLocal(url); r == nil {
		t.Errorf("OpenLocal(%s) failed", url)
	}
}

func TestThreadSafety(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir %v", err)
	}

	cmd := exec.Command("/bin/sh", "-euxc",
		strings.Join([]string{
			"cd " + dir,
			"git init",
			"touch file",
			"git add file",
			"git commit -m msg file",
		}, " && "))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Errorf("%s: %v", cmd.Args, err)
	}

	type res struct {
		err  error
		repo *git.Repository
	}

	// The following parallel code will generate TSAN errors if
	// libgit2 was not compiled with -DGIT_THREADS.
	done := make(chan res, 10)
	for i := 0; i < cap(done); i++ {
		go func() {
			repo, err := git.PlainOpen(dir)
			done <- res{err, repo}
		}()
	}

	for i := 0; i < cap(done); i++ {
		r := <-done
		if r.err != nil {
			t.Errorf("OpenRepository: %v", r.err)
		}
	}
}
