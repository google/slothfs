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
	"encoding/json"
	"reflect"
	"testing"
)

func TestTypes(t *testing.T) {
	type testCase struct {
		in   string
		want interface{}
		got  interface{}
	}

	cases := []testCase{
		{
			in: `{
  "id": "0d1df06d6de43086af19990f85b7b7c01799f984",
  "entries": [
    {
      "mode": 33188,
      "type": "blob",
      "id": "582b4959fa1f8e90330027291c612b1cebc4447c",
      "name": "index.html"
    }
  ]
}`,
			got: &Tree{},
			want: &Tree{
				ID: "0d1df06d6de43086af19990f85b7b7c01799f984",
				Entries: []TreeEntry{
					{
						Mode: 33188,
						Type: "blob",
						ID:   "582b4959fa1f8e90330027291c612b1cebc4447c",
						Name: "index.html",
					},
				},
			},
		},
		{
			in: `{
  "name": "gerrit",
  "clone_url": "file:///home/hanwen/vc/gerrit"
}`,
			want: &Project{Name: "gerrit",
				CloneURL: "file:///home/hanwen/vc/gerrit",
			},
			got: &Project{},
		},
		{
			in: `{
			"name": "Han-Wen Nienhuys",
			"email": "hanwen@google.com",
			"time": "Fri Feb 26 14:29:31 2016 +0100"
}`,
			want: &Person{
				Name:  "Han-Wen Nienhuys",
				Email: "hanwen@google.com",
				Time:  "Fri Feb 26 14:29:31 2016 +0100",
				//time.Date(2016, 2, 26, 14, 29, 31, 0, euTZ),
			},
			got: &Person{},
		},
		{
			in: `{
      "type": "modify",
      "old_id": "7d3569881e6dcc4745e853ecc8b9a570f3431060",
      "old_mode": 33188,
      "old_path": "polygerrit-ui/app/elements/gr-account-label.html",
      "new_id": "3f795d0d84716842d319fcf359b553e024a6b811",
      "new_mode": 33188,
      "new_path": "polygerrit-ui/app/elements/gr-account-label.html"
    }`,
			want: &DiffEntry{
				Type:    "modify",
				OldID:   "7d3569881e6dcc4745e853ecc8b9a570f3431060",
				OldMode: 33188,
				OldPath: "polygerrit-ui/app/elements/gr-account-label.html",
				NewID:   "3f795d0d84716842d319fcf359b553e024a6b811",
				NewMode: 33188,
				NewPath: "polygerrit-ui/app/elements/gr-account-label.html",
			},
			got: &DiffEntry{},
		},

		{
			in: `{
  "commit": "5378eff7b783acd83f2241983f9f97ccf9972d37",
  "tree": "868c42f4579291a85689c3def16cb146877af155",
  "parents": [
    "6233c1a23921c24be2c099fd21f7ea5e029e3777"
  ],
  "author": {
    "name": "Han-Wen Nienhuys",
    "email": "hanwen@google.com",
    "time": "Fri Feb 26 14:29:31 2016 +0100"
  },
  "committer": {
    "name": "Han-Wen Nienhuys",
    "email": "hanwen@google.com",
    "time": "Thu Mar 03 14:12:41 2016 +0100"
  },
  "message": "PolyGerrit: inject \"remove reviewer\" into gr-account-label\n\nThis makes gr-account-label into a \"chip\".\n\nBug: Issue \u003c3917\u003e\nChange-Id: Ibe4d62786b0625bad34e651b715a2085c4a2da51\n",
  "tree_diff": [
    {
      "type": "modify",
      "old_id": "7d3569881e6dcc4745e853ecc8b9a570f3431060",
      "old_mode": 33188,
      "old_path": "polygerrit-ui/app/elements/gr-account-label.html",
      "new_id": "3f795d0d84716842d319fcf359b553e024a6b811",
      "new_mode": 33188,
      "new_path": "polygerrit-ui/app/elements/gr-account-label.html"
    }
  ]
}`,
			want: &Commit{
				Commit: "5378eff7b783acd83f2241983f9f97ccf9972d37",
				Tree:   "868c42f4579291a85689c3def16cb146877af155",
				Parents: []string{
					"6233c1a23921c24be2c099fd21f7ea5e029e3777",
				},
				Author: Person{
					Name:  "Han-Wen Nienhuys",
					Email: "hanwen@google.com",
					Time:  "Fri Feb 26 14:29:31 2016 +0100",
				},
				Committer: Person{
					Name:  "Han-Wen Nienhuys",
					Email: "hanwen@google.com",
					Time:  "Thu Mar 03 14:12:41 2016 +0100",
				},
				Message: "PolyGerrit: inject \"remove reviewer\" into gr-account-label\n\nThis makes gr-account-label into a \"chip\".\n\nBug: Issue \u003c3917\u003e\nChange-Id: Ibe4d62786b0625bad34e651b715a2085c4a2da51\n",
				TreeDiff: []DiffEntry{
					{
						Type:    "modify",
						OldID:   "7d3569881e6dcc4745e853ecc8b9a570f3431060",
						OldMode: 33188,
						OldPath: "polygerrit-ui/app/elements/gr-account-label.html",
						NewID:   "3f795d0d84716842d319fcf359b553e024a6b811",
						NewMode: 33188,
						NewPath: "polygerrit-ui/app/elements/gr-account-label.html",
					},
				},
			},
			got: &Commit{},
		},
		{
			in: `{
  "log": [
    {
      "commit": "5378eff7b783acd83f2241983f9f97ccf9972d37",
      "tree": "868c42f4579291a85689c3def16cb146877af155",
      "parents": [
        "6233c1a23921c24be2c099fd21f7ea5e029e3777"
      ],
      "author": {
        "name": "Han-Wen Nienhuys",
        "email": "hanwen@google.com",
        "time": "Fri Feb 26 14:29:31 2016 +0100"
      },
      "committer": {
        "name": "Han-Wen Nienhuys",
        "email": "hanwen@google.com",
        "time": "Thu Mar 03 14:12:41 2016 +0100"
      },
      "message": "PolyGerrit: inject \"remove reviewer\" into gr-account-label\n\nThis makes gr-account-label into a \"chip\".\n\nBug: Issue \u003c3917\u003e\nChange-Id: Ibe4d62786b0625bad34e651b715a2085c4a2da51\n"
    }
  ],
  "next": "c32abd0104868f6d798348fa74cff420124deb0e"
}`,
			got: &Log{},
			want: &Log{
				Log: []Commit{
					{
						Commit: "5378eff7b783acd83f2241983f9f97ccf9972d37",
						Tree:   "868c42f4579291a85689c3def16cb146877af155",
						Parents: []string{
							"6233c1a23921c24be2c099fd21f7ea5e029e3777",
						}, Author: Person{
							Name:  "Han-Wen Nienhuys",
							Email: "hanwen@google.com",
							Time:  "Fri Feb 26 14:29:31 2016 +0100",
						},
						Committer: Person{
							Name:  "Han-Wen Nienhuys",
							Email: "hanwen@google.com",
							Time:  "Thu Mar 03 14:12:41 2016 +0100",
						},
						Message: "PolyGerrit: inject \"remove reviewer\" into gr-account-label\n\nThis makes gr-account-label into a \"chip\".\n\nBug: Issue \u003c3917\u003e\nChange-Id: Ibe4d62786b0625bad34e651b715a2085c4a2da51\n",
					},
				},
				Next: "c32abd0104868f6d798348fa74cff420124deb0e",
			},
		},

		{
			in: `{
      "start": 1,
      "count": 4,
      "path": "VERSION",
      "commit": "3765bb8add118be67b4b0ac6528661e66ac63264",
      "author": {
        "name": "Shawn Pearce",
        "email": "sop@google.com",
        "time": "2013-07-29 14:05:26 -0700"
      }
    }`,
			want: &BlameRegion{
				Start:  1,
				Count:  4,
				Path:   "VERSION",
				Commit: "3765bb8add118be67b4b0ac6528661e66ac63264",
				Author: Person{
					Name:  "Shawn Pearce",
					Email: "sop@google.com",
					Time:  "2013-07-29 14:05:26 -0700",
				},
			},
			got: &BlameRegion{},
		},
		{
			in: `{
   "plugins/oauth": {
    "name": "plugins/oauth",
    "clone_url": "https://gerrit.googlesource.com/plugins/oauth",
    "description": "OAuth provider for GitHub and Google",
    "branches": {
      "master": "56fbd4c28ba35877a38ec4c6bbfc6b5920db4207",
      "stable-2.10": "5d404e6c38ff380a3b8964c1343fb91b8ec6bb76"
    }
  }
}
`,
			want: &map[string]*Project{
				"plugins/oauth": &Project{
					Name:        "plugins/oauth",
					CloneURL:    "https://gerrit.googlesource.com/plugins/oauth",
					Description: "OAuth provider for GitHub and Google",
					Branches: map[string]string{
						"master":      "56fbd4c28ba35877a38ec4c6bbfc6b5920db4207",
						"stable-2.10": "5d404e6c38ff380a3b8964c1343fb91b8ec6bb76",
					}},
			},
			got: &map[string]*Project{},
		},
	}

	for _, c := range cases {
		if err := json.Unmarshal([]byte(c.in), c.got); err != nil {
			t.Errorf("%T: unmarshalGitiles: %v", c.want, err)
		} else if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%T: got %#v, want %#v", c.want, c.got, c.want)
		}
	}
}
