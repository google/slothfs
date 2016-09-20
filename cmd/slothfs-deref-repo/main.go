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

// This program creates a manifest file with revisions filled in from
// a normal repo checkout. This can be used for comparing slothfs and
// actual repo checkouts.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/google/slothfs/manifest"
	git "github.com/libgit2/git2go"
)

func main() {
	flag.Parse()
	if len(flag.Args()) == 0 {
		log.Fatal("must state the repo toplevel directory.")
	}

	top := flag.Arg(0)
	mf, err := manifest.ParseFile(filepath.Join(top, ".repo", "manifest.xml"))
	if err != nil {
		log.Fatal(err)
	}

	mf.Filter()
	for i, p := range mf.Project {
		repoPath := filepath.Join(top, p.GetPath(), ".git")
		repo, err := git.OpenRepository(repoPath)
		if err != nil {
			continue
		}

		h, err := repo.Head()
		if err != nil {
			log.Println("head", p.Name, err)
		}

		obj, err := h.Peel(git.ObjectCommit)
		if err != nil {
			log.Println("peel", p.Name, err)
		}

		mf.Project[i].Revision = obj.Id().String()
	}

	xml, err := mf.MarshalXML()
	if err != nil {
		log.Fatal("MarshalXML", err)
	}

	os.Stdout.Write(xml)
}
