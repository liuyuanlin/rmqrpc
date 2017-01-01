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
