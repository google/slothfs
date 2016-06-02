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
)

// Cache combines a blob, tree and git repo cache.
type Cache struct {
	git  *gitCache
	tree *TreeCache
	blob *CAS
}

func NewCache(d string) (*Cache, error) {
	d, err := filepath.Abs(d)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(d, 0700); err != nil {
		return nil, err
	}

	g, err := newGitCache(filepath.Join(d, "git"))
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

	return &Cache{git: g, tree: t, blob: c}, nil
}
