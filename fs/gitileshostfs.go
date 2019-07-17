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

package fs

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/gitiles"
	"github.com/hanwen/go-fuse/fs"
)

type hostFS struct {
	fs.Inode

	cache        *cache.Cache
	service      *gitiles.Service
	projects     map[string]*gitiles.Project
	cloneOptions []CloneOption
}

func parents(projMap map[string]*gitiles.Project) map[string]struct{} {
	dirs := map[string]struct{}{}
	for nm := range projMap {
		for nm != "" && nm != "." {
			next := filepath.Dir(nm)
			dirs[next] = struct{}{}
			nm = next
		}
	}
	return dirs
}

func NewHostFS(cache *cache.Cache, service *gitiles.Service, cloneOptions []CloneOption) (*hostFS, error) {
	projMap, err := service.List(nil)
	if err != nil {
		return nil, err
	}

	dirs := parents(projMap)
	for p := range projMap {
		if _, ok := dirs[p]; ok {
			return nil, fmt.Errorf("%q is a dir and a project", p)
		}
	}

	return &hostFS{
		projects:     projMap,
		cloneOptions: cloneOptions,
		service:      service,
		cache:        cache,
	}, nil
}

var _ = (fs.NodeOnAdder)((*hostFS)(nil))

func (h *hostFS) OnAdd(ctx context.Context) {
	var keys []string
	for k := range parents(h.projects) {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	nodes := map[string]*fs.Inode{
		"": h.EmbeddedInode(),
	}

	for _, k := range keys {
		if k == "." {
			continue
		}
		// XXX should loop ?
		d, nm := filepath.Split(k)
		d = strings.TrimSuffix(d, "/")
		parent := nodes[d]

		var node fs.InodeEmbedder
		if p := h.projects[k]; p != nil {
			node = h.newProjectNode(parent, p)
			delete(h.projects, k)
		} else {
			node = &fs.Inode{}
		}

		ch := parent.NewPersistentInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFDIR})
		parent.AddChild(nm, ch, true)
		nodes[k] = ch
	}

	for k, p := range h.projects {
		d, nm := filepath.Split(k)
		d = strings.TrimSuffix(d, "/")

		parent := nodes[d]
		node := h.newProjectNode(parent, p)

		ch := parent.NewPersistentInode(ctx, node, fs.StableAttr{Mode: syscall.S_IFDIR})
		parent.AddChild(nm, ch, true)
	}
}

func (h *hostFS) newProjectNode(parent *fs.Inode, proj *gitiles.Project) fs.InodeEmbedder {
	repoService := h.service.NewRepoService(proj.Name)
	opts := GitilesOptions{
		CloneURL:    proj.CloneURL,
		CloneOption: h.cloneOptions,
	}
	return NewGitilesConfigFSRoot(h.cache, repoService, &opts)
}
