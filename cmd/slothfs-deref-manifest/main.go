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

	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/populate"
)

func main() {
	gitilesOptions := gitiles.DefineFlags()
	branch := flag.String("branch", "master", "Specify branch of the manifest repository to use.")
	repo := flag.String("repo", "platform/manifest", "Set repository name holding manifest file.")
	flag.Parse()

	service, err := gitiles.NewService(*gitilesOptions)
	if err != nil {
		log.Fatalf("NewService: %v", err)
	}

	mf, err := populate.FetchManifest(service, *repo, *branch)
	if err != nil {
		log.Fatalf("FetchManifest: %v", err)
	}

	mf.Filter()

	if err := populate.DerefManifest(service, mf); err != nil {
		log.Fatalf("DerefManifest: %v", err)
	}

	xml, err := mf.MarshalXML()
	if err != nil {
		log.Fatalf("MarshalXML: %v", err)
	}

	os.Stdout.Write(xml)
}
