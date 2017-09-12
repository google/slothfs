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
	"sync"

	"gopkg.in/src-d/go-git.v4/plumbing"
)

type nodeCacheKey struct {
	ID   plumbing.Hash
	xbit bool
}

// The nodeCache keeps a map of ID to FS node. It is safe for
// concurrent use from multiple goroutines. The cache allows us to
// reuse out the same node for multiple files, effectively
// hard-linking the file. This is done for two reasons: first, each
// blob takes up kernel FS cache memory only once, even if it may be
// used in multiple checkouts. Second, moving data from the FUSE
// process into the kernel is relatively expensive. Thus, we can
// amortize the cost of the read over multiple checkouts.
type nodeCache struct {
	mu      sync.RWMutex
	nodeMap map[nodeCacheKey]*gitilesNode
}

func newNodeCache() *nodeCache {
	return &nodeCache{
		nodeMap: make(map[nodeCacheKey]*gitilesNode),
	}
}

func (c *nodeCache) get(id *plumbing.Hash, xbit bool) *gitilesNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.nodeMap[nodeCacheKey{*id, xbit}]
}

func (c *nodeCache) add(n *gitilesNode) {
	xbit := n.mode&0111 != 0
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nodeMap[nodeCacheKey{n.id, xbit}] = n
}
