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

package cache

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/src-d/go-git.v4/plumbing"
)

// CAS is a content addressable storage. It is intended to be used
// with git SHA1 data. It stores blobs as uncompressed files without
// git headers. This means that we can wire up files from the CAS
// directly with a FUSE file system.
type CAS struct {
	dir string
}

// NewCAS creates a new CAS object.
func NewCAS(dir string) (*CAS, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &CAS{
		dir: dir,
	}, nil
}

func (c *CAS) path(id plumbing.Hash) string {
	str := id.String()
	return fmt.Sprintf("%s/%s/%s", c.dir, str[:3], str[3:])
}

// Open returns a file corresponding to the blob, opened for reading.
func (c *CAS) Open(id plumbing.Hash) (*os.File, bool) {
	f, err := os.Open(c.path(id))
	return f, err == nil
}

// Write writes the given data under the given ID atomically.
func (c *CAS) Write(id plumbing.Hash, data []byte) error {
	// TODO(hanwen): we should run data through the git hash to
	// verify that it is what it says it is.
	f, err := ioutil.TempFile(c.dir, "tmp")
	if err != nil {
		return err
	}

	if err := f.Chmod(0444); err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	p := c.path(id)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.Rename(f.Name(), c.path(id))
}
