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

package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/gitfs/cache"
	"github.com/google/gitfs/fs"
	"github.com/google/gitfs/gitiles"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

func main() {
	url := flag.String("gitiles", "", "URL of gitiles service")
	branch := flag.String("branch", "master", "branch name")
	repo := flag.String("repo", "", "repository name")
	debug := flag.Bool("debug", false, "print debug info")
	cacheDir := flag.String("cache", filepath.Join(os.Getenv("HOME"), ".cache", "gitfs"), "cache dir")
	flag.Parse()

	if *cacheDir == "" {
		log.Fatal("must set --cache")
	}
	if len(flag.Args()) < 1 {
		log.Fatal("usage: main -gitiles URL -repo REPO [-branch BRANCH] MOUNT-POINT")
	}

	mntDir := flag.Arg(0)
	cache, err := cache.NewCache(*cacheDir)
	if err != nil {
		log.Printf("NewCache: %v", err)
	}

	service, err := gitiles.NewService(*url)
	if err != nil {
		log.Printf("NewService: %v", err)
	}

	repoService := service.NewRepoService(*repo)
	project, err := repoService.Get()
	if err != nil {
		log.Fatalf("GetProject(%s): %v", *repo, err)
	}

	tree, err := repoService.GetTree(*branch, "", true)
	if err != nil {
		log.Fatal(err)
	}

	opts := fs.GitilesOptions{
		Revision: *branch,
		CloneURL: project.CloneURL,
	}

	root := fs.NewGitilesRoot(cache, tree, repoService, opts)
	server, _, err := nodefs.MountRoot(mntDir, root, &nodefs.Options{
		EntryTimeout:    time.Hour,
		NegativeTimeout: time.Hour,
		AttrTimeout:     time.Hour,
	})
	if err != nil {
		log.Fatalf("MountFileSystem: %v", err)
	}
	if *debug {
		server.SetDebug(true)
	}
	log.Printf("Started gitiles fs FUSE on %s", mntDir)
	server.Serve()
}
