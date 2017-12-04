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

package gitiles

import (
	"bytes"
	"fmt"
)

// Project describes a repository
type Project struct {
	Name        string
	CloneURL    string            `json:"clone_url"`
	Description string            `json:"description"`
	Branches    map[string]string `json:"branches"`
}

// Person describes a committer or author.
type Person struct {
	Name  string
	Email string

	// TODO(hanwen): time.Time.
	Time string
}

// DiffEntry describes a file difference.
type DiffEntry struct {
	Type    string
	OldID   string `json:"old_id"`
	OldMode int    `json:"old_mode"`
	OldPath string `json:"old_path"`
	NewID   string `json:"new_id"`
	NewMode int    `json:"new_mode"`
	NewPath string `json:"new_path"`
}

// Commit describes a git commit.
type Commit struct {
	Commit    string
	Tree      string
	Parents   []string
	Author    Person
	Committer Person
	Message   string
	TreeDiff  []DiffEntry `json:"tree_diff"`
}

// Log holds the output of a revwalk.
type Log struct {
	Log  []Commit
	Next string
}

// BlameRegion represents a attribution of a file range.
type BlameRegion struct {
	Start  int
	Count  int
	Path   string
	Commit string
	Author Person
}

// Blame represents all of the BlameRegions in a file.
type Blame struct {
	Regions []BlameRegion
}

// TreeEntry holds a single entry in a tree.
type TreeEntry struct {
	Mode int
	Type string
	ID   string
	Name string

	// Optional
	Size   *int
	Target *string
}

func (e *TreeEntry) String() string {
	s := fmt.Sprintf("%06o %s %s %s", e.Mode, e.Type, e.ID, e.Name)
	if e.Size != nil {
		s += fmt.Sprintf(" %d", *e.Size)
	}
	if e.Target != nil {
		s += fmt.Sprintf(" => %s", *e.Target)
	}
	return s
}

// Tree holds a (possibly recursively expanded) tree.
type Tree struct {
	ID      string
	Entries []TreeEntry
}

func (t *Tree) String() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "tree %s {\n", t.ID)
	for _, e := range t.Entries {
		fmt.Fprintf(&buf, "  %s\n", e.String())
	}
	fmt.Fprintf(&buf, "}\n")
	return buf.String()

}

// A git reference
type RefData struct {
	// The value to which a reference points.
	Value string

	// If the value points to a tag, the commit that the tag points to.
	Peeled string

	// If the ref is symbolic, eg. HEAD, the ref to which it points.
	Target string
}
