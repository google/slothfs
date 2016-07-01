
This is a FUSE filesystem that provides light-weight, lazily downloaded,
read-only checkouts of Git repositories. It is intended for use with Android.


How to use
==========

To start the file system:

    go install github.com/google/slothfs/cmd/slothfs-multifs
    mkdir /tmp/mnt
    slothfs-multifs -gitiles https://android.googlesource.com/  /tmp/mnt &

To create a workspace "ws" corresponding to the latest manifest version

    go install github.com/google/slothfs/cmd/slothfs-expand-manifest
    slothfs-expand-manifest --gitiles https://android.googlesource.com/ \
       > /tmp/m.xml &&
    ln -s /tmp/m.xml /tmp/mnt/config/ws

To populate a checkout

    go install github.com/google/slothfs/cmd/slothfs-populate
    mkdir -p checkout/frameworks
    cd checkout/frameworks
    git clone https://android.googlesource.com/platform/frameworks/base
    cd ../
    slothfs-populate -ro /tmp/mnt/ws .

The filesystem daemon uses an on-disk cache, which by default is stored under
~/.cache/slothfs


Configuring
===========

The FUSE file system clones repositories on-demand. You can avoid cloning
altogether for repositories you know you don't need.  This is configured through
a JSON file.

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

DISCLAIMER
==========

This is not an official Google product.
