
SlothFS is a FUSE filesystem that provides light-weight, lazily downloaded,
read-only checkouts of manifest-based Git projects. It is intended for use with
Android.


How to use
==========

To start the file system:

    go install github.com/google/slothfs/cmd/slothfs-repofs
    mkdir /tmp/mnt
    slothfs-repofs /tmp/mnt &

To create a workspace "ws" corresponding to the latest manifest version

    go install github.com/google/slothfs/cmd/slothfs-deref-manifest
    slothfs-deref-manifest > /tmp/m.xml
    ln -s /tmp/m.xml /tmp/mnt/config/ws

More details can be found in the [manual](docs/manual.md).


DISCLAIMER
==========

This is not an official Google product.
