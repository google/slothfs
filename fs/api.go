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
	"regexp"

	"github.com/google/gitfs/manifest"
)

// CloneOption configures for which files we should trigger a git clone.
type CloneOption struct {
	RE    *regexp.Regexp
	Clone bool
}

// GitilesOptions configures the Gitiles filesystem.
type GitilesOptions struct {
	Revision string

	// If set, clone the repo on reads from here.
	CloneURL string

	// List of filename options. We use the first matching option
	CloneOption []CloneOption
}

// ManifestOptions holds options for a Manifest file system.
type ManifestOptions struct {
	Manifest        *manifest.Manifest
	RepoCloneOption []CloneOption
	FileCloneOption []CloneOption
}
