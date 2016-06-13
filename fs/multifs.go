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
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/google/gitfs/cache"
	"github.com/google/gitfs/gitiles"
	"github.com/google/gitfs/manifest"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

type multiManifestFSRoot struct {
	nodefs.Node
	cache   *cache.Cache
	fsConn  *nodefs.FileSystemConnector
	options MultiFSOptions
	gitiles *gitiles.Service
}

func (r *multiManifestFSRoot) OnMount(fsConn *nodefs.FileSystemConnector) {
	r.fsConn = fsConn
	r.Inode().NewChild("config", true, &configNode{
		Node: nodefs.NewDefaultNode(),
		root: r,
	})
}

func NewMultiFS(service *gitiles.Service, c *cache.Cache, options MultiFSOptions) *multiManifestFSRoot {
	r := &multiManifestFSRoot{
		Node:    nodefs.NewDefaultNode(),
		cache:   c,
		options: options,
		gitiles: service,
	}
	return r
}

type configNode struct {
	nodefs.Node
	root *multiManifestFSRoot
}

type configEntryNode struct {
	nodefs.Node
	link []byte
}

func (c *configEntryNode) GetAttr(out *fuse.Attr, f nodefs.File, ctx *fuse.Context) fuse.Status {
	out.Mode = fuse.S_IFLNK
	return fuse.OK
}

func (c *configEntryNode) Readlink(ctx *fuse.Context) ([]byte, fuse.Status) {
	return c.link, fuse.OK
}

func (c *configNode) Symlink(name, content string, ctx *fuse.Context) (*nodefs.Inode, fuse.Status) {
	mfBytes, err := ioutil.ReadFile(content)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}

	mf, err := manifest.Parse(mfBytes)
	if err != nil {
		log.Printf("Parse(%s): %v", content, err)
		return nil, fuse.EINVAL
	}

	options := ManifestOptions{
		Manifest:        mf,
		RepoCloneOption: c.root.options.RepoCloneOption,
		FileCloneOption: c.root.options.FileCloneOption,
	}
	fs, err := NewManifestFS(c.root.gitiles, c.root.cache, options)
	if err != nil {
		log.Printf("NewManifestFS(%s): %v", string(content), err)
		return nil, fuse.EIO
	}

	ch := c.root.Inode().NewChild(name, true, fs)
	if ch == nil {
		// TODO(hanwen): can this ever happen?
		return nil, fuse.EINVAL
	}

	config := c.Inode().NewChild(name, false, &configEntryNode{
		Node: nodefs.NewDefaultNode(),
		// This is sneaky, but it appears to work.
		link: []byte(filepath.Join("..", name, "manifest.xml")),
	})

	if err := fs.(*manifestFSRoot).onMount(c.root.fsConn); err != nil {
		log.Println("onMount(%s): %v", name, err)
		for k := range ch.Children() {
			ch.RmChild(k)
		}

		ch.NewChild("ERROR", false, &dataNode{nodefs.NewDefaultNode(), []byte(err.Error())})
	}

	return config, fuse.OK
}

// TODO(hanwen): implement configNode.Unlink

// TODO(hanwen): make sure content nodes are shared between workspaces.
