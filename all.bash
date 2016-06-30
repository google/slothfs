#!/bin/bash

for sub in manifest \
gitiles \
cache \
fs \
cmd/gitfs-expand-manifest \
cmd/gitfs-multifs \
cmd/gitfs-manifestfs \
cmd/gitfs-populate \
cmd/gitfs-gitilesfs \
cmd/gitfs-deref-manifest \
  ; do
  p=github.com/google/gitfs/${sub}
  go clean $p
  go test $p
  go install $p
done
