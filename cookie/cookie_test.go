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

package cookie

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestParseCookieJar(t *testing.T) {
	in := `# Netscape HTTP Cookie File
# http://www.netscape.com/newsref/std/cookie_spec.html
# This is a generated file!  Do not edit.
#HttpOnly_login.netscape.com	FALSE	/	FALSE	1467968199	XYZ     
#HttpOnly_login.netscape.com	FALSE	/	FALSE	1467968199	XYZ	abc|pqr`

	buf := bytes.NewBufferString(in)
	got, err := ParseCookieJar(buf)
	if err != nil {
		t.Fatalf("ParseCookieJar: %v", err)
	}

	want := []*http.Cookie{
		{
			Domain:   "login.netscape.com",
			Path:     "/",
			Secure:   false,
			Expires:  time.Unix(1467968199, 0),
			Name:     "XYZ",
			HttpOnly: true,
		},
		{
			Domain:   "login.netscape.com",
			Path:     "/",
			Secure:   false,
			Expires:  time.Unix(1467968199, 0),
			Name:     "XYZ",
			Value:    "abc|pqr",
			HttpOnly: true,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
