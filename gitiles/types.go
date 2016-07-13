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

import "fmt"

type Project struct {
	Name        string
	CloneURL    string            `json:"clone_url"`
	Description string            `json:"description"`
	Branches    map[string]string `json:"branches"`
}

type Person struct {
	Name  string
	Email string

	// TODO(hanwen): time.Time.
	Time string
}

type DiffEntry struct {
	Type    string
	OldID   string `json:"old_id"`
	OldMode int    `json:"old_mode"`
	OldPath string `json:"old_path"`
	NewID   string `json:"new_id"`
	NewMode int    `json:"new_mode"`
	NewPath string `json:"new_path"`
}

type Commit struct {
	Commit    string
	Tree      string
	Parents   []string
	Author    Person
	Committer Person
	Message   string
	TreeDiff  []DiffEntry `json:"tree_diff"`
}

type Log struct {
	Log  []Commit
	Next string
}

type BlameRegion struct {
	Start  int
	Count  int
	Path   string
	Commit string
	Author Person
}

type Blame struct {
	Regions []BlameRegion
}

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

type Tree struct {
	ID      string
	Entries []TreeEntry
}
