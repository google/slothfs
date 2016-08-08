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

// Package cache implements a simplistic persistent cache based on the
// filesystem.
package cache

import (
	"os"
	"path/filepath"
	"time"
)

// Cache combines a blob, tree and git repo cache.
type Cache struct {
	Git  *gitCache
	Tree *TreeCache
	Blob *CAS

	root string
}

// Options defines configurable options for the different caches.
type Options struct {
	// FetchFrequency controls how often we run git fetch on the
	// locally cached git repositories.
	FetchFrequency time.Duration
}

// NewCache sets up a Cache instance according to the given options.
func NewCache(d string, opts Options) (*Cache, error) {
	if opts.FetchFrequency == 0 {
		opts.FetchFrequency = 12 * time.Hour
	}

	d, err := filepath.Abs(d)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(d, 0700); err != nil {
		return nil, err
	}

	g, err := newGitCache(filepath.Join(d, "git"), opts)
	if err != nil {
		return nil, err
	}

	c, err := NewCAS(filepath.Join(d, "blobs"))
	if err != nil {
		return nil, err
	}

	t, err := NewTreeCache(filepath.Join(d, "tree"))
	if err != nil {
		return nil, err
	}

	return &Cache{Git: g, Tree: t, Blob: c,
		root: d,
	}, nil
}

// Root returns the directory holding the cache storage.
func (c *Cache) Root() string { return c.root }
