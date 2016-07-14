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

package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/google/slothfs/populate"
)

func main() {
	mount := flag.String("ro", "", "path to slothfs-multifs mount.")
	flag.Parse()

	dir := "."
	if len(flag.Args()) == 1 {
		dir = flag.Arg(0)
	} else if len(flag.Args()) > 1 {
		log.Fatal("too many arguments.")
	}

	added, changed, err := populate.Checkout(*mount, dir)
	if err != nil {
		log.Fatalf("populate.Checkout: %v", err)
	}

	if len(changed) > 0 {
		now := time.Now()
		n := 0
		for _, slice := range [][]string{added, changed} {
			for _, c := range slice {
				err := os.Chtimes(c, now, now)
				if os.IsNotExist(err) {
					fi, statErr := os.Lstat(c)
					if statErr == nil && fi.Mode()&os.ModeSymlink != 0 {
						// Ignore broken symlinks.
						err = nil
					}
				}
				if err != nil {
					log.Fatalf("Chtimes(%s): %v", c, err)
				}
				n++
			}
		}
		log.Printf("touched %d files", n)
	} else {
		log.Printf("no files were changed, %d were added; assuming fresh checkout.", len(added))
	}
}
