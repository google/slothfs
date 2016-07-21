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
	"encoding/json"
	"fmt"
	"regexp"
)

type configEntry struct {
	File  string
	Repo  string
	Clone bool
}

// ReadConfig reads a JSON file containing clone options
func ReadConfig(contents []byte) (repo []CloneOption, file []CloneOption, err error) {
	var cfg []configEntry
	if err := json.Unmarshal(contents, &cfg); err != nil {
		return nil, nil, err
	}

	for _, e := range cfg {
		if e.File != "" {
			re, err := regexp.Compile(e.File)
			if err != nil {
				return nil, nil, err
			}

			file = append(file, CloneOption{re, e.Clone})
		} else if e.Repo != "" {
			re, err := regexp.Compile(e.Repo)
			if err != nil {
				return nil, nil, err
			}

			repo = append(repo, CloneOption{re, e.Clone})

		} else {
			return nil, nil, fmt.Errorf("must set either File or Repo")
		}
	}

	return repo, file, nil
}
