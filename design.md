
Goal
====
Minimize source control overhead for Android developers


Background
==========

Android uses git repositories stitched together with ‘repo’. There are several
hundred repos, with new ones added very frequently. Managing these (syncing,
checking out) is a significant waste of time. There are avoidable
inefficiencies, because the tree contains a significant amount of unused data
for unused host platforms, unused target architectures/devices and unused
history.

Requirements
============

* Released as Open source
* Runs on Linux and Mac
* Both for automated and manual use
* Build and create CLs for the Android tree with minimal overhead
* Easily deployable

Idea
====
* Provide a lightweight mechanism to create a read-only snapshot of the Android tree.
* Provide tooling to check out some repositories as git, and create a symlink
  forest to the read-only snapshot for the rest.

Implementation
==============

Overview:

1. We provide a FUSE file system with R/O snapshots of the tree.
2. We provide tooling to complete a partial checkout with symlinks to a R/O
   snapshot.
3. We provide tooling to sync partial checkout, updating timestamps to
   satisfy the build system.

FUSE filesystem overview
------------------------

Provide a FUSE file system for the R/O snapshot.

   * The FS is read-only, except for timestamps.
   * The FS uses hardlinks for blobs with the same content.
   * The FS populates metadata from the Gitiles JSON REST API.
   * When a file from some repo is opened for reading, fetch the content. A heuristic will decide between:
      * Full clone (useful for code repositories)
      * Shallow clone (prebuilts: history doesn’t matter, but need full tree)
      * Individual object downloads from Gitiles (don’t need full tree)
   * The FS runs as the user

The FS can be configured by symlinking manifest.xml or a submodule commit SHA1
   * ln -s ~/android/.repo/manifest.xml /fuse/config/WORKSPACE-NAME
   * ln -s ~/submod-android:master /fuse/config/WORKSPACE-NAME

Optional optimizations:

   * Do periodic git pre-fetches from the FS; minimizes wait time when issuing the sync command.
   * Initialize from an existing repo installation
   * Create an on-disk Content Addressable Store for source files to minimize time to unpack data on startup.


Tooling overview
----------------
* Provide tooling to calculate diff between 2 trees. Based on this diff, call “touch” on the changed blobs.
* Provide tooling to populate a workspace with symlink forest pointing to the FUSE snapshot, and checking out individual subrepos as normal .git repositories.


Motivation
==========

Why OSX
-------

This is a request from Android team. It actually makes providing a good solution
more difficult, because OSX and OSXFUSE lack several features:

   * No attribute cache control
   * No kernel UnionFS or bind mounts.
   * OSXFUSE is buggy
   * OSXFUSE lacks performance optimizations (eg. readdirplus, zero roundtrip reads)

Why FUSE?
---------

   * FUSE is the only way to avoid downloading data for unused files
   * You can also create trees cheaply by hard/soft linking a CAS directly. However, it is easy for users to accidentally edit files in the CAS, leading to build breakage.
   * This could be circumvented by asking users to run the CAS under a different UID, but that is bothersome to set up.

Why readonly?
-------------

Write access goes through the normal file system.  This avoids surfacing a
writable tree in FUSE.

   * Git performance would suffer if routed through FUSE
   * A writable git repo would have to be backed by some data on disk. Using a normal git repo is easiest for troubleshooting and for users to understand, but at the same time, the standard posix interface (which uses filenames) is ill-suited to implement a posixly correct file system. We sidestep this problem by not offering a writable tree.
   * Preventing writes prevents FS race conditions.
   * For a r/o FS, we can set infinite timeouts on attributes and entry data,
     minimizing kernel roundtrips.

Why writable timestamps?
------------------------

We must support incremental builds, so syncs must lead to timestamp changes.

   * In OSXFUSE, the FS can’t invalidate attributes, so it is better to change timestamps from the outside.

Why hardlinks for blobs?
------------------------

   * multiple checkouts of similar trees can share kernel page cache memory for
     the trees.
   * reading through FUSE is expensive. Sharing the blobs amortizes reading costs.
   * disadvantage:  blobs are shared, so when setting timestamps for one
     workspace, other workspaces are affected too, leading to spurious rebuilds
     in those workspaces.

Why run FUSE as the user?
-------------------------

   * Simplifies credential management, in case the FS must contact authenticated services.
   * The FS doesn’t have to reason about the permissions and owners of the process opening some file.
   * Simplifies deployment. User does not need root privileges to use this.


Why use gitiles JSON API, and git wire protocol?
------------------------------------------------

   * Gitiles + git wire protocol are already supported in open-source Gerrit. Zero deployment overhead for external users.
   * Git wire protocol is battle tested and well optimized for bulk transfers.
   * The gitiles JSON API is sufficient for what we want.

Open questions
==============

* How should we integrate this into existing tooling? (repo?)
* Does ‘repo’ get confused when part of the checkout is a symlink forest?
* Do we replace (reimplement) repo, or extend it?

Implementation steps
====================

* Gitiles:
   * Add support for ‘size’ field to tree listings. The ‘size’ field is necessary for serving FUSE data.
   * Add support for recursive tree traversals. This saves roundtrips.

* FUSE daemon:
   * Add support for serving trees based on Gitiles JSON data
   * Add support for lazily cloning .git repositories
   * Add support for chtimes
   * Add support for sharing blobs
   * Add support for a CAS
   * Add support for surfacing manifest.xml with SHA1s.
   * Investigate bazil.org/fuse, which has better community support.

* Tooling:
   * Generate composite workspace of symlink forest and git repos
   * Generate chtime() calls based on two manifest.xml (with SHA1s) or submodule SHA1s.
   * Write a sync command
   * Add ‘checkout’ command
