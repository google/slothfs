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
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/google/slothfs/cache"
	"github.com/google/slothfs/gitiles"
	"github.com/google/slothfs/manifest"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

type multiManifestFSRoot struct {
	nodefs.Node
	nodeCache *nodeCache
	cache     *cache.Cache
	fsConn    *nodefs.FileSystemConnector
	options   MultiManifestFSOptions
	gitiles   *gitiles.Service
}

func (r *multiManifestFSRoot) StatFs() *fuse.StatfsOut {
	var s syscall.Statfs_t
	err := syscall.Statfs(r.cache.Root(), &s)
	if err == nil {
		out := &fuse.StatfsOut{}
		out.FromStatfsT(&s)
		return out
	}
	return nil
}

func (c *configNode) configureWorkspaces() error {
	if c.root.options.ManifestDir == "" {
		return nil
	}
	fs, err := filepath.Glob(filepath.Join(c.root.options.ManifestDir, "*"))
	if err != nil || len(fs) == 0 {
		return err
	}

	log.Println("configuring workspaces...")
	var wg sync.WaitGroup
	wg.Add(len(fs))
	for _, f := range fs {
		go func(n string) {
			_, code := c.Symlink(filepath.Base(n), n, nil)
			log.Printf("manifest %s: %v", n, code)
			wg.Done()
		}(f)
	}
	wg.Wait()

	return nil
}

func (r *multiManifestFSRoot) OnMount(fsConn *nodefs.FileSystemConnector) {
	r.fsConn = fsConn

	cfg := &configNode{
		Node: nodefs.NewDefaultNode(),
		root: r,
	}
	r.Inode().NewChild("config", true, cfg)

	if err := cfg.configureWorkspaces(); err != nil {
		log.Printf("configureWorkspaces: %v", err)
	}
}

func (c *configNode) Deletable() bool { return false }

func NewMultiManifestFS(service *gitiles.Service, c *cache.Cache, options MultiManifestFSOptions) *multiManifestFSRoot {
	r := &multiManifestFSRoot{
		Node:      nodefs.NewDefaultNode(),
		nodeCache: newNodeCache(),
		cache:     c,
		options:   options,
		gitiles:   service,
	}
	return r
}

func (r *multiManifestFSRoot) Deletable() bool { return false }

func (r *multiManifestFSRoot) GetXAttr(attribute string, context *fuse.Context) (data []byte, code fuse.Status) {
	return nil, fuse.ENODATA
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

func (c *configEntryNode) Deletable() bool { return false }

func (c *configNode) Unlink(name string, ctx *fuse.Context) fuse.Status {
	child := c.root.Inode().RmChild(name)
	if child == nil {
		return fuse.ENOENT
	}

	// Notify the kernel this part of the tree disappeared.
	c.root.fsConn.DeleteNotify(c.root.Inode(), child, name)

	c.Inode().RmChild(name)

	// No need to notify for the removed symlink. Since we're in
	// the Unlink method, will VFS already knows about the
	// deletion once we return OK.

	if dir := c.root.options.ManifestDir; dir != "" {
		os.Remove(filepath.Join(dir, name))
	}

	return fuse.OK
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
	fs.(*manifestFSRoot).nodeCache = c.root.nodeCache

	child := c.root.Inode().NewChild(name, true, fs)
	if child == nil {
		// TODO(hanwen): can this ever happen?
		return nil, fuse.EINVAL
	}

	config := c.Inode().NewChild(name, false, &configEntryNode{
		Node: nodefs.NewDefaultNode(),
		// This is sneaky, but it appears to work.
		link: []byte(filepath.Join("..", name, ".slothfs", "manifest.xml")),
	})

	if err := fs.(*manifestFSRoot).onMount(c.root.fsConn); err != nil {
		log.Printf("onMount(%s): %v", name, err)
		for k := range child.Children() {
			child.RmChild(k)
		}

		child.NewChild("ERROR", false, &dataNode{nodefs.NewDefaultNode(), []byte(err.Error())})
	} else {
		if dir := c.root.options.ManifestDir; dir != "" {
			for {
				f, err := ioutil.TempFile(dir, "")
				if err != nil {
					break
				}

				_, err = f.Write(mfBytes)
				if err != nil {
					break
				}

				if err := f.Close(); err != nil {
					break
				}

				os.Rename(f.Name(), filepath.Join(dir, name))
				break
			}
		}
	}

	c.root.fsConn.EntryNotify(c.root.Inode(), name)

	return config, fuse.OK
}
