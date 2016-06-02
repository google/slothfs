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

	git "github.com/libgit2/git2go"
)

// lazyRepo represents a git repository that might be fetched on
// demand.
type lazyRepo struct {
	allowClone bool
	url        string
	cache      *gitCache

	repoMu  sync.Mutex
	cloning bool
	repo    *git.Repository
}

func newLazyRepo(url string, cache *gitCache, allowClone bool) *lazyRepo {
	r := &lazyRepo{
		url:        url,
		cache:      cache,
		allowClone: allowClone,
	}
	return r
}

// Repository returns a git.Repository for this repo, or nil if it
// wasn't loaded.  This method is safe for concurrent use from
// multiple goroutines.
func (r *lazyRepo) Repository() *git.Repository {
	r.repoMu.Lock()
	defer r.repoMu.Unlock()
	return r.repo
}

// runClone initiates a clone. It makes sure that only one clone
// process runs at any time.
func (r *lazyRepo) runClone() {
	repo, err := r.cache.Open(r.url)

	r.repoMu.Lock()
	defer r.repoMu.Unlock()
	r.allowClone = false
	r.cloning = false
	r.repo = repo

	if err != nil {
		log.Printf("runClone: %v", err)
	}
}

// Clone schedules the repository to be cloned.  This method is safe
// for concurrent use from multiple goroutines.
func (r *lazyRepo) Clone() {
	r.repoMu.Lock()
	defer r.repoMu.Unlock()
	if !r.allowClone || r.repo != nil {
		return
	}

	if r.cloning {
		return
	}
	r.cloning = true
	go r.runClone()
}
