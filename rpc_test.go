// Copyright 2013 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rmqrpc_test

import (
	"errors"
	"log"
	"net/rpc"
	"testing"
	"time"

	"github.com/liuyuanlin/rmqrpc"

	msg "github.com/liuyuanlin/rmqrpc/examples/message.pb"
)

type Arith int

func (t *Arith) Add(args *msg.ArithRequest, reply *msg.ArithResponse) error {
	reply.C = args.A + args.B
	log.Printf("Arith.Add(%v, %v): %v", args.A, args.B, reply.C)
	return nil
}

func (t *Arith) Mul(args *msg.ArithRequest, reply *msg.ArithResponse) error {
	reply.C = args.A * args.B
	return nil
}

func (t *Arith) Div(args *msg.ArithRequest, reply *msg.ArithResponse) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}
	reply.C = args.A / args.B
	return nil
}

func (t *Arith) Error(args *msg.ArithRequest, reply *msg.ArithResponse) error {
	return errors.New("ArithError")
}

type Echo int

func (t *Echo) Echo(args *msg.EchoRequest, reply *msg.EchoResponse) error {
	reply.Msg = args.Msg
	return nil
}

func TestInternalMessagePkg(t *testing.T) {
	err := listenAndServeArithAndEchoService("amqp://guest:guest@localhost:5672/", "", "rpc_queue")
	if err != nil {
		log.Fatalf("listenAndServeArithAndEchoService: %v", err)
	}

	client := rpc.NewClientWithCodec(rmqrpc.NewClientCodec("amqp://guest:guest@localhost:5672/", "", "rpc_queue"))
	defer client.Close()

	testArithClient(t, client)
	testEchoClient(t, client)

	testArithClientAsync(t, client)
	testEchoClientAsync(t, client)
}

func listenAndServeArithAndEchoService(uri string, exchangeName string, queueName string) error {

	srv := rpc.NewServer()
	if err := srv.RegisterName("ArithService", new(Arith)); err != nil {
		return err
	}
	if err := srv.RegisterName("EchoService", new(Echo)); err != nil {
		return err
	}
	go srv.ServeCodec(rmqrpc.NewServerCodec(uri, exchangeName, queueName))

	return nil
}

func testArithClient(t *testing.T, client *rpc.Client) {
	var args msg.ArithRequest
	var reply msg.ArithResponse
	var err error

	// Add
	args.A = 1
	args.B = 2
	if err = client.Call("ArithService.Add", &args, &reply); err != nil {
		t.Fatalf(`arith.Add: %v`, err)
	}
	if reply.C != 3 {
		t.Fatalf(`arith.Add: expected = %d, got = %d`, 3, reply.C)
	}

	// Mul
	args.A = 2
	args.B = 3
	if err = client.Call("ArithService.Mul", &args, &reply); err != nil {
		t.Fatalf(`arith.Mul: %v`, err)
	}
	if reply.C != 6 {
		t.Fatalf(`arith.Mul: expected = %d, got = %d`, 6, reply.C)
	}

	// Div
	args.A = 13
	args.B = 5
	if err = client.Call("ArithService.Div", &args, &reply); err != nil {
		t.Fatalf(`arith.Div: %v`, err)
	}
	if reply.C != 2 {
		t.Fatalf(`arith.Div: expected = %d, got = %d`, 2, reply.C)
	}

	// Div zero
	args.A = 1
	args.B = 0
	if err = client.Call("ArithService.Div", &args, &reply); err.Error() != "divide by zero" {
		t.Fatalf(`arith.Error: expected = "%s", got = "%s"`, "divide by zero", err.Error())
	}

	// Error
	args.A = 1
	args.B = 2
	if err = client.Call("ArithService.Error", &args, &reply); err.Error() != "ArithError" {
		t.Fatalf(`arith.Error: expected = "%s", got = "%s"`, "ArithError", err.Error())
	}
}

func testArithClientAsync(t *testing.T, client *rpc.Client) {
	done := make(chan *rpc.Call, 16)
	callInfoList := []struct {
		method string
		args   *msg.ArithRequest
		reply  *msg.ArithResponse
		err    error
	}{
		{
			"ArithService.Add",
			&msg.ArithRequest{A: 1, B: 2},
			&msg.ArithResponse{C: 3},
			nil,
		},
		{
			"ArithService.Mul",
			&msg.ArithRequest{A: 2, B: 3},
			&msg.ArithResponse{C: 6},
			nil,
		},
		{
			"ArithService.Div",
			&msg.ArithRequest{A: 13, B: 5},
			&msg.ArithResponse{C: 2},
			nil,
		},
		{
			"ArithService.Div",
			&msg.ArithRequest{A: 1, B: 0},
			&msg.ArithResponse{},
			errors.New("divide by zero"),
		},
		{
			"ArithService.Error",
			&msg.ArithRequest{A: 1, B: 2},
			&msg.ArithResponse{},
			errors.New("ArithError"),
		},
	}

	// GoCall list
	calls := make([]*rpc.Call, len(callInfoList))
	for i := 0; i < len(calls); i++ {
		calls[i] = client.Go(callInfoList[i].method,
			callInfoList[i].args, callInfoList[i].reply,
			done,
		)
	}
	for i := 0; i < len(calls); i++ {
		<-calls[i].Done
	}

	// check result
	for i := 0; i < len(calls); i++ {
		if callInfoList[i].err != nil {
			if calls[i].Error.Error() != callInfoList[i].err.Error() {
				t.Fatalf(`%s: expected %v, Got = %v`,
					callInfoList[i].method,
					callInfoList[i].err,
					calls[i].Error,
				)
			}
			continue
		}

		got := calls[i].Reply.(*msg.ArithResponse).C
		expected := callInfoList[i].reply.C
		if got != expected {
			t.Fatalf(`%v: expected %v, Got = %v`,
				callInfoList[i].method, got, expected,
			)
		}
	}
}

func testEchoClient(t *testing.T, client *rpc.Client) {
	var args msg.EchoRequest
	var reply msg.EchoResponse
	var err error

	// EchoService.Echo
	args.Msg = "Hello, Protobuf-RPC"
	if err = client.Call("EchoService.Echo", &args, &reply); err != nil {
		t.Fatalf(`EchoService.Echo: %v`, err)
	}
	if reply.Msg != args.Msg {
		t.Fatalf(`EchoService.Echo: expected = "%s", got = "%s"`, args.Msg, reply.Msg)
	}
}

func testEchoClientAsync(t *testing.T, client *rpc.Client) {
	// EchoService.Echo
	args := &msg.EchoRequest{Msg: "Hello, Protobuf-RPC"}
	reply := &msg.EchoResponse{}
	echoCall := client.Go("EchoService.Echo", args, reply, nil)

	// sleep 1s
	time.Sleep(time.Second)

	// EchoService.Echo reply
	echoCall = <-echoCall.Done
	if echoCall.Error != nil {
		t.Fatalf(`EchoService.Echo: %v`, echoCall.Error)
	}
	if echoCall.Reply.(*msg.EchoResponse).Msg != args.Msg {
		t.Fatalf(`EchoService.Echo: expected = "%s", got = "%s"`,
			args.Msg,
			echoCall.Reply.(*msg.EchoResponse).Msg,
		)
	}
}
