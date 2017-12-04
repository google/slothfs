// Copyright 2017 Google Inc. All rights reserved.
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

package gitiles

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"testing"
)

func TestProductionArchive(t *testing.T) {
	gs, err := NewService(Options{
		Address: "https://go.googlesource.com",
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	repo := gs.NewRepoService("crypto")
	if err != nil {
		t.Fatalf("NewRepoService: %v", err)
	}

	stream, err := repo.GetArchive("master", "ssh", ArchiveTgz)
	if err != nil {
		t.Fatalf("GetArchive: %v", err)
	}
	defer stream.Close()

	gz, err := gzip.NewReader(stream)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	r := tar.NewReader(gz)

	names := map[string]bool{}
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		names[hdr.Name] = true
	}

	if !names["mux.go"] {
		t.Fatal("did not find 'mux.go', got %v", names)
	}
}
