
SlothFS is a FUSE filesystem that provides light-weight, lazily downloaded,
read-only checkouts of manifest-based Git projects. It is intended for use with
Android.


How to use
==========

To start the file system:

    go install github.com/google/slothfs/cmd/slothfs-multifs
    mkdir /tmp/mnt
    slothfs-multifs -gitiles https://android.googlesource.com/ /tmp/mnt &

To create a workspace "ws" corresponding to the latest manifest version

    go install github.com/google/slothfs/cmd/slothfs-expand-manifest
    slothfs-expand-manifest --gitiles https://android.googlesource.com/ \
       > /tmp/m.xml &&
    ln -s /tmp/m.xml /tmp/mnt/config/ws

More details can be found in the [manual](docs/manual.md).


DISCLAIMER
==========

This is not an official Google product.
