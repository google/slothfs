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

// slothfs-gitiles-test is a program executing a single gitiles HTTP
// request, to be used for troubleshooting proxy/auth problems.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/google/slothfs/gitiles"
)

func main() {
	tap := flag.Bool("tap", false, "Tap traffic exchanged with $http_proxy")
	gitilesOptions := gitiles.DefineFlags()
	flag.Parse()

	if *tap {
		tapTraffic()
	}
	service, err := gitiles.NewService(*gitilesOptions)
	if err != nil {
		log.Fatalf("NewService: %v", err)
	}

	projs, err := service.List(nil)
	if err != nil {
		log.Fatalf("List: %v", err)
	}

	for p := range projs {
		fmt.Printf("project: %s\n", p)
	}
}

func logCopy(w io.Writer, r io.Reader, who string) {
	var buf [320000]byte

	for {
		n, e1 := r.Read(buf[:])
		log.Println(who, string(buf[:n]))
		_, e2 := w.Write(buf[:n])
		if e1 != nil || e2 != nil {
			break
		}
	}
}

func forward(conn net.Conn, addr string) {
	f, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		logCopy(f, conn, "A")
		wg.Done()
	}()
	go func() {
		logCopy(conn, f, "B")
		wg.Done()
	}()
	wg.Wait()
	f.Close()
	conn.Close()
}

func tapTraffic() {
	proxy := os.Getenv("http_proxy")
	if proxy == "" {
		log.Println("no http_proxy, not tapping")
		return
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
	}
	os.Setenv("http_proxy", l.Addr().String())

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				break
			}
			go forward(c, proxy)
		}
	}()
}
