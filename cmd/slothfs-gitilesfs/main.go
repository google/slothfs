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

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/fs"
	"github.com/google/slothfs/gitiles"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

func main() {
	repo := flag.String("repo", "", "Set the repository name.")
	debug := flag.Bool("debug", false, "Print FUSE debug info.")
	cacheDir := flag.String("cache", filepath.Join(os.Getenv("HOME"), ".cache", "slothfs"),
		"Set directory for file system cache.")
	gitilesOptions := gitiles.DefineFlags()
	flag.Parse()

	if *cacheDir == "" {
		log.Fatal("must set --cache")
	}
	if len(flag.Args()) < 1 {
		log.Fatal("usage: main -repo REPO MOUNT-POINT")
	}

	mntDir := flag.Arg(0)
	cache, err := cache.NewCache(*cacheDir, cache.Options{})
	if err != nil {
		log.Fatalf("NewCache: %v", err)
	}

	service, err := gitiles.NewService(*gitilesOptions)
	if err != nil {
		log.Fatalf("NewService: %v", err)
	}

	repoService := service.NewRepoService(*repo)
	project, err := repoService.Get()
	if err != nil {
		log.Fatalf("GetProject(%s): %v", *repo, err)
	}

	opts := fs.GitilesOptions{
		CloneURL: project.CloneURL,
	}

	root := fs.NewGitilesConfigFSRoot(cache, repoService, &opts)
	server, _, err := nodefs.MountRoot(mntDir, root, &nodefs.Options{
		EntryTimeout:    time.Hour,
		NegativeTimeout: time.Hour,
		AttrTimeout:     time.Hour,
		Debug:           *debug,
	})
	if err != nil {
		log.Fatalf("MountFileSystem: %v", err)
	}
	log.Printf("Started gitiles fs FUSE on %s", mntDir)
	server.Serve()
}
