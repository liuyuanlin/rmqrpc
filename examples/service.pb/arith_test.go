// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"log"
	"sync"
)

var (
	arithHost = "127.0.0.1"
	arithPort = 2010

	onceArith sync.Once
)

func setupArithServer() {
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	go func() {
		wg.Done()
		err := StartArithServiceServer("amqp://guest:guest@localhost:5672/", "", "rpc_queue", new(Arith))
		if err != nil {
			log.Fatalf("StartServeArithService: %v", err)
		}
	}()
}
