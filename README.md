
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


DISCLAIMER
==========

This is not an official Google product.
