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
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

func main() {
	cacheDir := flag.String("cache", filepath.Join(os.Getenv("HOME"), ".cache", "slothfs"),
		"Set the directory holding the filesystem cache.")
	debug := flag.Bool("debug", false, "Print FUSE debug info")
	config := flag.String("config", filepath.Join(os.Getenv("HOME"), ".config", "slothfs"),
		"Set the directory with configuration files.")
	gitilesOptions := gitiles.DefineFlags()
	flag.Parse()

	if *cacheDir == "" {
		log.Fatal("must set --cache")
	}

	if len(flag.Args()) < 1 {
		log.Fatal("mountpoint argument missing.")
	}

	mntDir := flag.Arg(0)

	cache, err := cache.NewCache(*cacheDir, cache.Options{})
	if err != nil {
		log.Printf("NewCache: %v", err)
	}

	service, err := gitiles.NewService(*gitilesOptions)
	if err != nil {
		log.Printf("NewService: %v", err)
	}

	opts := fs.MultiManifestFSOptions{}
	if *config != "" {
		cloneJS := filepath.Join(*config, "clone.json")
		configContents, err := ioutil.ReadFile(cloneJS)
		if err != nil {
			log.Fatal(err)
		}
		opts.RepoCloneOption, opts.FileCloneOption, err = fs.ReadConfig(configContents)
		if err != nil {
			log.Fatal(err)
		}

		opts.ManifestDir = filepath.Join(*config, "manifests")
		if err := os.MkdirAll(opts.ManifestDir, 0755); err != nil {
			log.Fatal(err)
		}
	}

	root := fs.NewMultiManifestFS(service, cache, opts)
	nodeFSOpts := &nodefs.Options{
		EntryTimeout:    time.Hour,
		NegativeTimeout: time.Hour,
		AttrTimeout:     time.Hour,
		Debug:           *debug,
	}
	conn := nodefs.NewFileSystemConnector(root, nodeFSOpts)

	mountOpts := fuse.MountOptions{
		Name:   "slothfs",
		FsName: "slothfs",
		Debug:  *debug,
	}

	server, err := fuse.NewServer(conn.RawFS(), mntDir, &mountOpts)
	if err != nil {
		log.Fatalf("NewServer: %v", err)
	}

	log.Printf("Started SlothFS on %s", mntDir)
	server.Serve()
}
