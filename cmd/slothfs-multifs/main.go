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
	"io/ioutil"
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
	gitilesURL := flag.String("gitiles", "", "gitiles URL. If unset, derive from manifest location.")
	cacheDir := flag.String("cache", filepath.Join(os.Getenv("HOME"), ".cache", "slothfs"), "cache dir")
	debug := flag.Bool("debug", false, "print debug info")
	config := flag.String("config", "", "JSON file configuring what repositories should be cloned.")
	flag.Parse()

	if *cacheDir == "" {
		log.Fatal("must set --cache")
	}
	if *gitilesURL == "" {
		log.Fatal("must set --gitiles")
	}

	if len(flag.Args()) < 1 {
		log.Fatal("mountpoint argument missing.")
	}

	mntDir := flag.Arg(0)

	cache, err := cache.NewCache(*cacheDir, cache.Options{})
	if err != nil {
		log.Printf("NewCache: %v", err)
	}

	service, err := gitiles.NewService(*gitilesURL, gitiles.Options{})
	if err != nil {
		log.Printf("NewService: %v", err)
	}

	opts := fs.MultiFSOptions{}
	if *config != "" {
		configContents, err := ioutil.ReadFile(*config)
		if err != nil {
			log.Fatal(err)
		}
		opts.RepoCloneOption, opts.FileCloneOption, err = fs.ReadConfig(configContents)
		if err != nil {
			log.Fatal(err)
		}
	}

	root := fs.NewMultiFS(service, cache, opts)
	if err != nil {
		log.Fatalf("NewManifestFS: %v", err)
	}

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
	log.Printf("Started Git MultiFS FUSE on %s", mntDir)
	server.Serve()
}
