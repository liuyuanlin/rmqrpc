# rmqrpc
rmqrpc 实现了go语言基于rabbitmq为数据通道和protobuf为载体的远程调用。远程调用框架使用了go 自带的net/rpc.   rmqrpc 仅仅添加了处理通信和载体相关的ClientCodec 和 ServerCodec以及protobuf rpc 的简单代码生成。

# 特别说明
本项目是在https://github.com/chai2010/protorpc 进行改动的， 特别感谢chai2010开源protorpc。本项目也视为是protorpc 的rabbitmq分支。

# Install
## 1.Install rabbitmq
安装rabbitmq 最新版本RabbitMQ 3.6.6 release
参考官网 http://www.rabbitmq.com/download.html

## 2.Install `protorpc` package:

1. `go get github.com/liuyuanlin/rmqrpc`
2. `go run hello.go`

## 3.Install `protoc-gen-go` plugin:

1. install `protoc` at first: http://github.com/google/protobuf/releases
2. `go get github.com/liuyuanlin/rmqrpc/protoc-gen-go`
3. `go generate github.com/liuyuanlin/protorpc/examples/service.pb`
4. `go test github.com/liuyuanlin/protorpc/examples/service.pb`



# Examples

First, create [echo.proto](https://github.com/liuyuanlin/rmqrpc/blob/master/examples/service.pb/echo.proto):

```Proto
syntax = "proto3";

package service;

message EchoRequest {
	string msg = 1;
}

message EchoResponse {
	string msg = 1;
}

service EchoService {
	rpc Echo (EchoRequest) returns (EchoResponse);
	rpc EchoTwice (EchoRequest) returns (EchoResponse);
}
```

Second, generate [echo.pb.go](https://github.com/liuyuanlin/rmqrpc/blob/master/examples/service.pb/echo.pb.go) from [echo.proto]

`protoc --plugin=protoc-gen-go=../../protoc-gen-go/protoc-gen-go --go_out=plugins=rmqrpc:. echo.proto arith.proto`


Now, we can use the stub code like this:

```Go
package main

package main

import (
	"fmt"
	"log"

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
```
