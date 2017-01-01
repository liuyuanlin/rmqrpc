// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ingore

package main

import (
	"fmt"
	"log"

	//	"github.com/liuyuanlin/rmqrpc"

	service "github.com/liuyuanlin/rmqrpc/examples/service.pb"
)

type Echo int

func (t *Echo) Echo(args *service.EchoRequest, reply *service.EchoResponse) error {
	reply.Msg = args.Msg
	return nil
}

func (t *Echo) EchoTwice(args *service.EchoRequest, reply *service.EchoResponse) error {
	reply.Msg = args.Msg + args.Msg
	return nil
}

func init() {
	go service.StartEchoServiceServer("amqp://guest:guest@localhost:5672/", "", "rpc_queue", new(Echo))
}

func main() {
	echoClient := service.NewEchoServiceClient("amqp://guest:guest@localhost:5672/", "", "rpc_queue")
	if echoClient == nil {
		log.Fatalf("service.NewEchoServiceClient:fail")
	}
	defer echoClient.Close()

	args := &service.EchoRequest{Msg: "你好, 世界!"}
	reply, err := echoClient.EchoTwice(args)
	if err != nil {
		log.Fatalf("echoClient.EchoTwice: %v", err)
	}
	fmt.Println(reply.Msg)

	// Output:
	// 你好, 世界!你好, 世界!
}
