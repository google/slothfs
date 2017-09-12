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

package populate

import (
	"encoding/hex"
	"fmt"
	"log"

	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/manifest"
)

func parseID(s string) (*plumbing.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("parseID(%q): %v", s, err)
	}
	if len(b) != 20 {
		return nil, fmt.Errorf("hash must be 20 hex bytes")
	}

	var h plumbing.Hash
	copy(h[:], b)
	return &h, nil
}

func gitID(s string) *plumbing.Hash {
	h, err := parseID(s)
	if err != nil {
		log.Panic(err)
	}
	return h
}

// FetchManifest gets the default manifest file from a Gitiles server.
func FetchManifest(service *gitiles.Service, repo, branch string) (*manifest.Manifest, error) {
	project := service.NewRepoService(repo)

	// When checking this out, it's called "manifest.xml". Go figure.
	c, err := project.GetBlob(branch, "default.xml")
	if err != nil {
		return nil, err
	}
	mf, err := manifest.Parse(c)
	if err != nil {
		return nil, err
	}

	return mf, nil
}

// DerefManifest uses the Gitiles JSON interface to fill in
// Project.Revision and Project.CloneURL in the given manifest.
func DerefManifest(service *gitiles.Service, mf *manifest.Manifest) error {
	// Collect all branch names we might care about, so we can
	// request data from all branches in one JSON call.  Normally,
	// all projects use the same branch, but individual projects
	// may specify a special branch.
	branchSet := map[string]struct{}{}

	var todoProjects []int
	for i, p := range mf.Project {
		rev := mf.ProjectRevision(&p)

		// According to the repo doc, the revision should be a branch,
		// either like 'refs/heads/master' or 'master'. We abuse this field by
		// also allowing commit SHA1s.
		if _, err := parseID(rev); err == nil {
			// Already a SHA1, don't change.
			continue
		}

		branchSet[rev] = struct{}{}
		todoProjects = append(todoProjects, i)
	}

	var branches []string
	for k := range branchSet {
		branches = append(branches, k)
	}

	repos, err := service.List(branches)
	if err != nil {
		return err
	}
	for _, i := range todoProjects {
		p := &mf.Project[i]

		proj, ok := repos[p.Name]
		if !ok {
			return fmt.Errorf("server list doesn't mention repo %s", p.Name)
		}

		p.CloneURL = proj.CloneURL

		branch := mf.ProjectRevision(p)
		commit, ok := proj.Branches[branch]
		if !ok {
			return fmt.Errorf("branch %q for repo %s not returned", branch, p.Name)
		}

		p.Revision = commit
	}
	return nil
}
