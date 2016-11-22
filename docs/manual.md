
Building Android with SlothFS
=============================

[Slothfs](https://github.com/google/slothfs) is a FUSE file system that offers a
read-only view of a Git tree. It downloads files lazily, so it takes up less
diskspace than a full checkout based on `repo`. Once its caches are seeded,
creating fresh workspaces or syncing should take on the order of seconds.

It is developed as open source, and can be used by anyone that uses
Gerrit/Gitiles based Git hosting. The [design doc](design.md) explains the
background behind its design choices.

It has been tested on Linux, but we expect it works on OSX too.

For the remainder of this document, we will assume that you are trying to
compile some flavor of Android.


Installing
==========

Get the source code,

    go get github.com/google/slothfs/cmd/slothfs-repofs

SlothFS depends git2go, which depends on libgit2.  For git2go, we recommend
compiling libgit2 in statically, as documented
[here](https://github.com/libgit2/git2go#from-next).

To install all components, run

    cd $GOPATH/src/github.com/google/slothfs/ && sh all.bash

The rest of this document assumes this has been done, and `$GOPATH/bin/` is in
your `$PATH`.

In addition, install the standard Android `clone.json` to avoid unnecessary git
clones

    mkdir -p $HOME/.config/slothfs
    curl https://raw.githubusercontent.com/google/slothfs/master/android.json \
      >  $HOME/.config/slothfs/clone.json


Selecting the host
==================

The defaults of slothfs are for the public version of Android at
`android.googlesource.com`.  Set the `-gitiles_url` option to change against
which version of Android you want to run.


Mounting the filesystem
=======================

First, create a mountpoint:

    sudo mkdir -p /slothfs && sudo chown $USER /slothfs

Then, to mount the file system, run

    slothfs-repofs /slothfs


Dereferencing a manifest
========================

The manifest describes which repositories go into an Android checkout. To make a
file system out of this, we must decide at which exact revision each repository
should be offered.

This can be done with `slothfs-deref-manifest`, eg.

    slothfs-deref-manifest > /tmp/m.xml


Configuring a workspace
=======================

A workspace can be created by a symlinking a deferenced manifest field into the
`config` directory, i.e.

    ln -s /tmp/m.xml /slothfs/config/my-workspace

This should create a directory `/slothfs/my-workspace` holding the tree
described in `/tmp/m.xml`.

On the first time you do this, slothfs will have to fetch the tree data, which
is slow, so this might take a while.


Using a workspace
=================

To use the read-only tree, create a writable checkout. Let's assume we want to
change the `art` repo:

    mkdir -p checkout ; cd checkout
    git clone https://android.googlesource.com/platform/art

To make `checkout` into a full-fledged checkout, run

    slothfs-populate -ro /slothfs/my-workspace .

This populate the `checkout` directory with symlinks into `/slothfs`, yielding a
full check out.

If there were symlinks to a previous checkout in the workspace, this will also
update timestamps to make incremental builds work.


Syncing
=======

Advancing your checkout to a different timestamp uses the same commands. To sync
to the current state of the Android tree, do the following

    slothfs-populate -sync .

This fetches the latest manifest file, finds the project revisions, sets up a
workspace for the manifest, and updates the symlinks from your read/write
checkout.


Removing a workspace
====================

Remove a workspace by removing its symlink configuration entry, eg.

    rm /slothfs/config/my-workspace

Unmounting slothfs
==================

A SlothFS daemon can be unmounted with `fusermount`

     fusermount -u /slothfs

If this reports `device busy`, check if there any processes holding open files
(eg. Jack compilation servers.)


Metadata
========

The file system offers the following metadata files:

     workspace/.slothfs/manifest.xml - manifest XML

     workspace/path/to/repo/.slothfs/tree.json - tree listing of this repository
     workspace/path/to/repo/.slothfs/treeID - hex tree ID of the this repository

In addition, each blob has the `user.gitsha1` extended attribute that surfaces
the blob's git SHA1 checksum.


Configuring
===========

SlothFS clones repositories on-demand, as soon as a file in the repository is
opened. You can avoid cloning altogether for repositories you know you don't
need; the files will then be fetched over HTTP.

Details of which repositories and files trigger cloning are configured through a
JSON file.

For example, if you work on Android, and build on a Linux machine, you will
never need the Darwin related prebuilts. You can avoid a costly clone for those
by doing:

    {"Repo": ".*darwin.*", "Clone": false}

Similarly, the build system system will read files (typically called '*.mk')
across the entire tree. When any .mk file is opened, this should not trigger a
clone. This is achieved with the following entry

    {"File": ".*mk$", "Clone": false}

Together, the following `config.json` file is a good start for working on
android:

    [{"Repo": ".*darwin.*", "Clone": false},
     {"File": ".*mk$", "Clone": false}]

A more elaborate configuration file is included as `android.json`.


File layout
-----------

SlothFS loads the configuration data from a directory that can be ste with the
`-config` flag. The following configuration is available

    $HOME/.config/clone.json   # clone configuration
    $HOME/.config/manifests/   # configured workspaces

SlothFS caches data in a directory which can be set with `-cache` flag.
The following data are cached:

    $HOME/.cache/slothfs/tree  # trees
    $HOME/.cache/slothfs/git   # bare git repositories
    $HOME/.cache/slothfs/blob  # blobs


Caveats: timestamps
-------------------

When syncing a checkout with `slothfs-populate`, an attempt is made to update
timestamps of the blobs, so incremental builds will work as expected. However,
since blobs are shared between different workspaces, a sync in one workspace may
cause spurious rebuilds in other workspaces.

Similarly, interrupting `slothfs-populate` and then syncing to another workspace
may yield unpredictable results.

When the slothfs FUSE daemon is restarted, all timestamp information is lost.
