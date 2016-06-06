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
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/google/gitfs/cache"
	"github.com/google/gitfs/gitiles"
	"github.com/google/gitfs/manifest"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	git "github.com/libgit2/git2go"
)

type manifestFSRoot struct {
	nodefs.Node

	service *gitiles.Service

	cache *cache.Cache
	trees map[string]*gitiles.Tree

	options ManifestOptions
}

// NewManifestFS creates a Manifest FS root node.
func NewManifestFS(service *gitiles.Service, cache *cache.Cache, opts ManifestOptions) (nodefs.Node, error) {
	root := &manifestFSRoot{
		Node:    nodefs.NewDefaultNode(),
		cache:   cache,
		service: service,
		options: opts,
	}

	for _, p := range opts.Manifest.Project {
		if _, err := git.NewOid(p.Revision); err != nil {
			return nil, fmt.Errorf("project %s revision %q does not parse: %v", p.Name, p.Revision, err)
		}
	}

	var err error
	root.trees, err = fetchTreeMap(cache.Tree, service, opts.Manifest)
	if err != nil {
		return nil, err
	}
	return root, nil
}

func (fs *manifestFSRoot) OnMount(fsConn *nodefs.FileSystemConnector) {
	if err := fs.onMount(fsConn); err != nil {
		log.Printf("onMount: %v", err)
		for k := range fs.Inode().Children() {
			fs.Inode().RmChild(k)
		}

		fs.Inode().NewChild("ERROR", false, newDataNode(err.Error()))
	}

	// Don't need the trees anymore.
	fs.trees = nil
}

func (fs *manifestFSRoot) onMount(fsConn *nodefs.FileSystemConnector) error {
	var byDepth [][]string
	for p := range fs.trees {
		d := len(strings.Split(p, "/"))
		for len(byDepth) <= d {
			byDepth = append(byDepth, nil)
		}

		byDepth[d] = append(byDepth[d], p)
	}

	revmap := map[string]*manifest.Project{}
	for i, p := range fs.options.Manifest.Project {
		revmap[p.Path] = &fs.options.Manifest.Project[i]
	}

	for _, ps := range byDepth {
		for _, p := range ps {
			dir, base := filepath.Split(p)
			parent, left := fsConn.Node(fs.Inode(), dir)
			for _, c := range left {
				ch := parent.NewChild(c, true, nodefs.NewDefaultNode())
				parent = ch
			}

			clone := true
			for _, e := range fs.options.RepoCloneOption {
				if e.RE.FindString(p) != "" {
					clone = e.Clone
					break
				}
			}

			cloneURL := revmap[p].CloneURL
			if !clone {
				cloneURL = ""
			}

			repoService := fs.service.NewRepoService(revmap[p].Name)

			opts := GitilesOptions{
				Revision:    revmap[p].Revision,
				CloneURL:    cloneURL,
				CloneOption: fs.options.FileCloneOption,
			}

			subRoot := NewGitilesRoot(fs.cache, fs.trees[p], repoService, opts)
			parent.NewChild(base, true, subRoot)
			if err := subRoot.(*gitilesRoot).onMount(fsConn); err != nil {
				return fmt.Errorf("onMount(%s): %v", p, err)
			}
		}
	}

	return nil
}

func fetchTreeMap(treeCache *cache.TreeCache, service *gitiles.Service, mf *manifest.Manifest) (map[string]*gitiles.Tree, error) {
	type resultT struct {
		path string
		resp *gitiles.Tree
		err  error
	}

	// TODO(hanwen): if we have the repository, and commit
	// locally, we should use cache.GetTree() instead of putting
	// load on the remote Gitiles.

	// Fetch all the trees in parallel.
	out := make(chan resultT, len(mf.Project))
	for _, p := range mf.Project {
		go func(p manifest.Project) {
			revID, err := git.NewOid(p.Revision)
			if err != nil {
				out <- resultT{p.Path, nil, err}
				return
			}

			tree, err := treeCache.Get(revID)
			if err != nil {
				repoService := service.NewRepoService(p.Name)

				tree, err = repoService.GetTree(p.Revision, "", true)
				if err == nil {
					if err := treeCache.Add(revID, tree); err != nil {
						log.Printf("treeCache.Add: %v", err)
					}
				}
			}

			out <- resultT{p.Path, tree, err}
		}(p)
	}

	// drain goroutines
	var result []resultT
	for range mf.Project {
		r := <-out
		result = append(result, r)
	}

	//
	resmap := map[string]*gitiles.Tree{}
	for _, r := range result {
		if r.err != nil {
			return nil, fmt.Errorf("Tree(%s): %v", r.path, r.err)
		}

		resmap[r.path] = r.resp
	}
	return resmap, nil
}
