#!/bin/bash

# Copyright (C) 2016 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


for sub in manifest \
  gitiles \
  cache \
  fs \
  populate \
cmd/slothfs-deref-manifest \
cmd/slothfs-repofs \
cmd/slothfs-manifestfs \
cmd/slothfs-populate \
cmd/slothfs-gitilesfs \
cmd/slothfs-deref-repo \
cmd/slothfs-gitiles-test \
  ; do
  p=github.com/google/slothfs/${sub}
  go clean $p
  go test $p
  go install $p
done
