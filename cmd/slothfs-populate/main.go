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
	"bufio"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/populate"
)

// findSlothFSMount guesses where slothfs might be mounted.
func findSlothFSMount() string {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		// We're probably on OSX.
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, " ")
		if len(fields) >= 3 && fields[2] == "fuse.slothfs" {
			return fields[1]
		}
	}
	return ""
}

// syncManifest fetches a manifest file, and configures a workspace
// for it.
func syncManifest(opts *gitiles.Options, mountPoint, repo, branch string) (string, error) {
	service, err := gitiles.NewService(*opts)
	if err != nil {
		return "", err
	}

	mf, err := populate.FetchManifest(service, repo, branch)
	if err != nil {
		return "", err
	}

	mf.Filter()

	if err := populate.DerefManifest(service, mf); err != nil {
		return "", err
	}

	xml, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}

	xmlBytes, err := mf.MarshalXML()
	if err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(xml.Name(), xmlBytes, 0644); err != nil {
		return "", err
	}

	name := strings.Replace(time.Now().Format("S"+time.RFC3339), ":", "_", -1)

	log.Printf("fetched manifest; configuring workspace %s", name)
	if err := os.Symlink(xml.Name(), filepath.Join(mountPoint, "config", name)); err != nil {
		return "", err
	}

	return filepath.Join(mountPoint, name), nil
}

func main() {
	gitilesOptions := gitiles.DefineFlags()
	newROWorkspace := flag.String("ro", "", "Set path to slothfs-repofs mount.")
	mount := flag.String("mount", "", "Set slothfs mountpoint for -sync option. Autodetected if empty.")
	sync := flag.Bool("sync", false, "Sync checkout to latest manifest version.")
	syncBranch := flag.String("sync_branch", "master", "Use this branch for -sync.")
	syncRepo := flag.String("sync_repo", "platform/manifest", "Use this repo for -sync.")
	flag.Parse()

	dir := "."
	if len(flag.Args()) == 1 {
		dir = flag.Arg(0)
	} else if len(flag.Args()) > 1 {
		log.Fatal("too many arguments.")
	}

	if *sync {
		if *mount == "" {
			*mount = findSlothFSMount()
			if *mount == "" {
				log.Fatal("could not autodetect mount point. Pass --mount option.")
			}
		}

		var err error
		*newROWorkspace, err = syncManifest(gitilesOptions, *mount, *syncRepo, *syncBranch)
		if err != nil {
			log.Fatalf("syncManifest: %v", err)
		}
	}

	if *newROWorkspace == "" {
		log.Fatalf("no readonly checkout given. Specify -ro DIR or -sync.")
	}

	log.Printf("creating symlinks to %s", *newROWorkspace)

	added, changed, err := populate.Checkout(*newROWorkspace, dir)
	if err != nil {
		log.Fatalf("populate.Checkout: %v", err)
	}

	if len(changed) > 0 {
		now := time.Now()
		n := 0
		for _, slice := range [][]string{added, changed} {
			for _, c := range slice {
				err := os.Chtimes(c, now, now)
				if os.IsNotExist(err) {
					fi, statErr := os.Lstat(c)
					if statErr == nil && fi.Mode()&os.ModeSymlink != 0 {
						// Ignore broken symlinks.
						err = nil
					}
				}
				if err != nil {
					log.Fatalf("Chtimes(%s): %v", c, err)
				}
				n++
			}
		}
		log.Printf("touched %d files", n)
	} else {
		log.Printf("no files were changed, %d were added; assuming fresh checkout.", len(added))
	}
}
