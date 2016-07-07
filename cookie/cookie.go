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

// cookie parses curl cookie jar files.
package cookie

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ParseCookieJar parses a cURL/Mozilla/Netscape cookie jar text file.
func ParseCookieJar(r io.Reader) ([]*http.Cookie, error) {
	var result []*http.Cookie
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		httpOnly := false
		const httpOnlyPrefix = "#HttpOnly_"
		if strings.HasPrefix(line, httpOnlyPrefix) {
			line = line[len(httpOnlyPrefix):]
			httpOnly = true
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			return nil, fmt.Errorf("got %d fields in line %q, want 8", len(fields), line)
		}

		exp, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return nil, err
		}

		c := http.Cookie{
			Domain:   fields[0],
			Name:     fields[5],
			Value:    fields[6],
			Path:     fields[2],
			Expires:  time.Unix(exp, 0),
			Secure:   fields[3] == "TRUE",
			HttpOnly: httpOnly,
		}

		result = append(result, &c)
	}

	return result, nil
}

func NewJar(path string) (http.CookieJar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cs, err := ParseCookieJar(f)
	if err != nil {
		return nil, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	for _, c := range cs {
		jar.SetCookies(&url.URL{
			Scheme: "http",
			Host:   c.Domain,
		}, []*http.Cookie{c})
	}

	return jar, nil
}
