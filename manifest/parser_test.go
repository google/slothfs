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

package manifest

import (
	"reflect"
	"testing"
)

func newString(s string) *string {
	return &s
}

var aospManifest = `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
  <remote  name="aosp"
           fetch=".."
           review="https://android-review.googlesource.com/" />
  <default revision="master"
           remote="aosp"
           sync-j="4" />

  <project path="build" name="platform/build" groups="pdk,tradefed" >
    <copyfile src="core/root.mk" dest="Makefile" />
  </project>
  <project path="build/soong" name="platform/build/soong" groups="pdk,tradefed" >
    <linkfile src="root.bp" dest="Android.bp" />
  </project>
</manifest>`

func TestBasic(t *testing.T) {
	manifest, err := Parse([]byte(aospManifest))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	want := &Manifest{
		Remote: []Remote{{
			Name:   "aosp",
			Fetch:  "..",
			Review: "https://android-review.googlesource.com/",
		}},
		Default: Default{
			Revision: "master",
			Remote:   "aosp",
			SyncJ:    "4",
		},
		Project: []Project{
			{
				Path:         newString("build"),
				Name:         "platform/build",
				GroupsString: "pdk,tradefed",
				Groups: map[string]bool{
					"pdk":      true,
					"tradefed": true,
				},
				Copyfile: []Copyfile{
					{
						Src:  "core/root.mk",
						Dest: "Makefile",
					},
				},
			},
			{
				Path:         newString("build/soong"),
				Name:         "platform/build/soong",
				GroupsString: "pdk,tradefed",
				Groups: map[string]bool{
					"pdk":      true,
					"tradefed": true,
				},
				Linkfile: []Linkfile{
					{
						Src:  "root.bp",
						Dest: "Android.bp",
					},
				},
			},
		},
	}

	if !reflect.DeepEqual(manifest, want) {
		t.Errorf("got %v, want %v", manifest, want)
	}
}

func TestRoundtrip(t *testing.T) {
	manifest, err := Parse([]byte(aospManifest))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	xml, err := manifest.MarshalXML()
	if err != nil {
		t.Errorf("MarshalXML: %v", err)
	}

	roundtrip, err := Parse(xml)
	if err != nil {
		t.Errorf("Parse(roundtrip): %v", err)
	}

	if !reflect.DeepEqual(roundtrip, manifest) {
		t.Errorf("got roundtrip %#v, want %#v", roundtrip, manifest)
	}
}
