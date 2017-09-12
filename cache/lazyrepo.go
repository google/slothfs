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
	"log"
	"sync"

	git "gopkg.in/src-d/go-git.v4"
)

// LazyRepo represents a git repository that might be fetched on
// demand.
type LazyRepo struct {
	url   string
	cache *gitCache

	repoMu  sync.Mutex
	cloning bool
	repo    *git.Repository
}

func newLazyRepo(url string, cache *gitCache) *LazyRepo {
	r := &LazyRepo{
		url:   url,
		cache: cache,
		repo:  cache.OpenLocal(url),
	}

	return r
}

// NewLazyRepo creates a new repository. If the repository is never to
// be cloned, url should be set to empty string.
func NewLazyRepo(url string, cache *Cache) *LazyRepo {
	return newLazyRepo(url, cache.Git)
}

// Repository returns a git.Repository for this repo, or nil if it
// wasn't loaded. This method is safe for concurrent use from
// multiple goroutines. The return value must not be Free'd since it
// is persisted inside LazyRepo.
func (r *LazyRepo) Repository() *git.Repository {
	r.repoMu.Lock()
	defer r.repoMu.Unlock()
	return r.repo
}

// runClone initiates a clone. It makes sure that only one clone
// process runs at any time.
func (r *LazyRepo) runClone() {
	repo, err := r.cache.Open(r.url)

	r.repoMu.Lock()
	defer r.repoMu.Unlock()
	r.url = ""
	r.cloning = false
	r.repo = repo

	if err != nil {
		log.Printf("runClone: %v", err)
	}
}

// Clone schedules the repository to be cloned.  This method is safe
// for concurrent use from multiple goroutines.
func (r *LazyRepo) Clone() {
	r.repoMu.Lock()
	defer r.repoMu.Unlock()
	if r.url == "" || r.repo != nil {
		return
	}

	if r.cloning {
		return
	}
	r.cloning = true
	go r.runClone()
}
