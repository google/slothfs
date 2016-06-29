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
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	git "github.com/libgit2/git2go"
)

// gitCache manages a set of bare git repositories.
type gitCache struct {
	// directory to hold the repositories.
	dir string

	// Directory to store log files for fetches and clones.
	logDir string
}

// newGitCache constructs a gitCache object.
func newGitCache(baseDir string) (*gitCache, error) {
	c := gitCache{
		dir:    filepath.Join(baseDir),
		logDir: filepath.Join(baseDir, "gitfs-logs"),
	}
	if err := os.MkdirAll(c.logDir, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return nil, err
	}
	return &c, nil
}

// logfile returns a logfile open for writing with a unique name.
func (c *gitCache) logfile() (*os.File, error) {
	nm := fmt.Sprintf("%s/git.%s.log", c.logDir, time.Now().Format(time.RFC3339Nano))
	nm = strings.Replace(nm, ":", "_", -1)
	return os.Create(nm)
}

// Fetch updates the local clone of the given repository.
func (c *gitCache) Fetch(url string) error {
	path, err := c.gitPath(url)
	if err != nil {
		return err
	}
	if err := c.runGit(c.dir, "--git-dir="+path, "fetch", "origin"); err != nil {
		return err
	}

	return nil
}

// gitPath transforms a URL into a path under the gitCache directory.
func (c *gitCache) gitPath(u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return "", err
	}

	p := path.Clean(parsed.Path)
	if path.Base(p) == ".git" {
		p = path.Dir(p)
	}
	return filepath.Join(c.dir, parsed.Host, p+".git"), nil
}

// runGit runs git with the given arguments under the given directory.
func (c *gitCache) runGit(dir string, args ...string) error {
	logfile, err := c.logfile()
	if err != nil {
		return err
	}
	defer logfile.Close()

	cmd := exec.Command("git", args...)
	log.Printf("running %s (log: %s)", cmd.Args, logfile.Name())
	cmd.Dir = dir

	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	runErr := cmd.Run()

	if _, err := fmt.Fprintf(logfile, "args: %s\ndir:%s\nEXIT: %s\n\nOUT\n%s\n\nERR\n\n", cmd.Args,
		cmd.Dir, out.String(), errOut.String()); err != nil {
		return fmt.Errorf("logfile write for %s (%v): %v",
			args, runErr, err)
	}

	if err := logfile.Close(); err != nil {
		return fmt.Errorf("logfile close for %s (%v): %v",
			args, runErr, err)
	}

	log.Printf("ran %s exit %v", cmd.Args, err)
	return runErr
}

// OpenLocal returns an opened repository for the given URL, if it is available locally.
func (c *gitCache) OpenLocal(url string) *git.Repository {
	p, err := c.gitPath(url)
	if err != nil {
		return nil
	}

	repo, err := git.OpenRepository(p)
	if err != nil {
		return nil
	}
	return repo
}

// Open returns an opened repository for the given URL. If necessary,
// the repository is cloned.
func (c *gitCache) Open(url string) (*git.Repository, error) {
	// TODO(hanwen): multiple concurrent calls to Open() with the
	// same URL may race, resulting in a double clone. It's unclear
	// what will happen in that case.
	p, err := c.gitPath(url)
	if err != nil {
		return nil, err
	}

	if _, err := os.Lstat(p); os.IsNotExist(err) {
		dir, base := filepath.Split(p)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		if err := c.runGit(dir, "clone", "--bare", "--progress", "--verbose", url, base); err != nil {
			return nil, err
		}
	}
	repo, err := git.OpenRepository(p)
	return repo, err
}
