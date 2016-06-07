
This is a FUSE filesystem that provides light-weight, read-only checkouts of
Android.


How to use
==========

To start,

    go install github.com/google/gitfs/cmd/gitfs-{multifs,expand-manifest}
    gitfs-expand-manifest --gitiles https://android.googlesource.com/ \
       > /tmp/m.xml
    mkdir /tmp/mnt
    gitfs-multifs -cache /tmp/cache -gitiles https://android.googlesource.com/  /tmp/mnt &

then, in another terminal, execute

    ln -s /tmp/m.xml /tmp/mnt/config/ws

To create a workspace "ws" corresponding to the manifest in m.xml.


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
     {"File": ".*mk$", "Clone": true}]


DISCLAIMER
==========

This is not an official Google product.
