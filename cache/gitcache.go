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
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	git "gopkg.in/src-d/go-git.v4"
)

// gitCache manages a set of bare git repositories.  Repositories are
// recognized by URLs. Port numbers in git URLs are not part of the
// cache key.
type gitCache struct {
	// directory to hold the repositories.
	dir string

	// Directory to store log files for fetches and clones.
	logDir string
}

// newGitCache constructs a gitCache object.
func newGitCache(baseDir string, opts Options) (*gitCache, error) {
	c := gitCache{
		dir:    filepath.Join(baseDir),
		logDir: filepath.Join(baseDir, "slothfs-logs"),
	}
	if err := os.MkdirAll(c.logDir, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return nil, err
	}
	if opts.FetchFrequency > 0 {
		go c.recurringFetch(opts.FetchFrequency)
	}

	return &c, nil
}

func (c *gitCache) recurringFetch(freq time.Duration) {
	ticker := time.NewTicker(freq)
	for {
		if err := c.FetchAll(); err != nil {
			log.Printf("FetchAll: %v", err)
		}
		<-ticker.C
	}
}

// logfile returns a logfile open for writing with a unique name.
func (c *gitCache) logfile() (*os.File, error) {
	nm := fmt.Sprintf("%s/git.%s.log", c.logDir, time.Now().Format(time.RFC3339Nano))
	nm = strings.Replace(nm, ":", "_", -1)
	return os.Create(nm)
}

// Fetch updates the local clone of the given repository.
func (c *gitCache) Fetch(dir string) error {
	if err := c.runGit(c.dir, "--git-dir="+dir, "fetch", "origin"); err != nil {
		return err
	}

	return nil
}

// FetchAll finds all known repos and runs git-fetch on them.
func (c *gitCache) FetchAll() error {
	dir, err := filepath.EvalSymlinks(c.dir)
	if err != nil {
		return err
	}

	var dirs []string
	if err := filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
		if fi.IsDir() && strings.HasSuffix(n, ".git") {
			dirs = append(dirs, n)
			return filepath.SkipDir
		}
		return nil
	}); err != nil {
		return err
	}

	for _, d := range dirs {
		if err := c.Fetch(d); err != nil {
			return fmt.Errorf("fetch %s: %v", d, err)
		}
	}

	return nil
}

// gitPath transforms a URL into a path under the gitCache directory.
func (c *gitCache) gitPath(u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return "", err
	}

	if h, _, err := net.SplitHostPort(parsed.Host); err == nil {
		parsed.Host = h
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

	if err != nil {
		log.Printf("ran %s exit %v", cmd.Args, err)
	}
	return runErr
}

// OpenLocal returns an opened repository for the given URL, if it is available locally.
func (c *gitCache) OpenLocal(url string) *git.Repository {
	p, err := c.gitPath(url)
	if err != nil {
		return nil
	}
	repo, err := git.PlainOpen(p)
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
	repo, err := git.PlainOpen(p)
	return repo, err
}
