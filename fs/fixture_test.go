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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/gitiles"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

type testFixture struct {
	dir        string
	mntDir     string
	server     *fuse.Server
	cache      *cache.Cache
	testServer *testServer
	service    *gitiles.Service
	root       nodefs.Node
}

func (f *testFixture) cleanup() {
	if f.testServer != nil {
		f.testServer.listener.Close()
	}
	if f.server != nil {
		f.server.Unmount()
	}
	os.RemoveAll(f.dir)
}

func newTestFixture() (*testFixture, error) {
	d, err := ioutil.TempDir("", "slothfs")
	if err != nil {
		return nil, err
	}

	fixture := &testFixture{dir: d}

	fixture.cache, err = cache.NewCache(filepath.Join(d, "/cache"), cache.Options{})
	if err != nil {
		return nil, err
	}

	fixture.testServer, err = newTestServer()
	if err != nil {
		return nil, err
	}

	fixture.service, err = gitiles.NewService(gitiles.Options{
		Address: fmt.Sprintf("http://%s", fixture.testServer.addr),
	})
	if err != nil {
		return nil, err
	}

	return fixture, nil
}

func (f *testFixture) mount(root nodefs.Node) error {
	f.mntDir = filepath.Join(f.dir, "mnt")
	if err := os.Mkdir(f.mntDir, 0755); err != nil {
		return err
	}

	fuseOpts := &nodefs.Options{
		EntryTimeout:    time.Hour,
		NegativeTimeout: time.Hour,
		AttrTimeout:     time.Hour,
	}

	var err error
	f.server, _, err = nodefs.MountRoot(f.mntDir, root, fuseOpts)
	if err != nil {
		return err
	}

	if fuseDebug {
		f.server.SetDebug(true)
	}
	go f.server.Serve()

	f.root = root
	return nil
}
