// Copyright 2013 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rmqrpc

import (
	"errors"
	"fmt"
	"hash/crc32"
	"log"
	"net/rpc"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	wire "github.com/liuyuanlin/rmqrpc/wire.pb"
	"github.com/streadway/amqp"
)

type requestAttr struct {
	Id      uint64
	ReplyTo string
}

type serverCodec struct {
	Uri          string
	ExchangeName string
	QueueName    string
	AmqpConnect  *amqp.Connection
	AmqpChannel  *amqp.Channel
	AmqpQueue    amqp.Queue
	AmqMsgs      <-chan amqp.Delivery

	// temporary work space
	reqHeader wire.RequestHeader

	// Package rpc expects uint64 request IDs.
	// We assign uint64 sequence numbers to incoming requests
	// but save the original request ID in the pending map.
	// When rpc responds, we use the sequence number in
	// the response to find the original request ID.
	mutex   sync.Mutex // protects seq, pending
	seq     uint64
	pending map[uint64]requestAttr
}

// NewServerCodec returns a serverCodec that communicates with the ClientCodec
// on the other end of the given conn.
func NewServerCodec(uri string, exchangeName string, queueName string) rpc.ServerCodec {
	return &serverCodec{
		Uri:          uri,
		ExchangeName: exchangeName,
		QueueName:    queueName,
		pending:      make(map[uint64]requestAttr),
	}
}

func (c *serverCodec) readyQueue() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	log.Println("start connect broker")
	var err error
	if c.AmqpConnect == nil {
		c.AmqpConnect, err = amqp.Dial(c.Uri)
		if err != nil {
			log.Println("amqp dial error:%v", err)
			return err
		}
	}
	log.Println("connect broker success")
	if c.AmqpChannel == nil {
		c.AmqpChannel, err = c.AmqpConnect.Channel()
		if err != nil {
			log.Println("amqp channel error:%v", err)
			return err
		}
	}
	if c.AmqpQueue.Name == "" {
		log.Println("AmqpQueue  create")
		c.AmqpQueue, err = c.AmqpChannel.QueueDeclare(c.QueueName, false, false, false, false, nil)
		if err != nil {
			log.Println("amqp Queue Declare error:%v", err)
			return err
		}
		c.AmqMsgs, err = c.AmqpChannel.Consume(
			c.AmqpQueue.Name, // queue
			"",               // consumer
			true,             // auto-ack
			false,            // exclusive
			false,            // no-local
			false,            // no-wait
			nil,              // args
		)
		if err != nil {
			log.Println("Consume fail :", err)
			return err
		}
	}

	return nil
}

func (c *serverCodec) ReadRequestHeader(r *rpc.Request) error {
	err := c.readyQueue()
	if err != nil {
		return err
	}
	header := wire.RequestHeader{}
	msg := <-c.AmqMsgs
	// Marshal Header
	err = proto.Unmarshal(msg.Body, &header)
	if err != nil {
		return err
	}

	if msg.ReplyTo == "" {
		return errors.New("reply to is nil")
	}

	c.mutex.Lock()
	c.seq++
	c.pending[c.seq] = requestAttr{Id: header.Id, ReplyTo: msg.ReplyTo}
	r.ServiceMethod = header.Method
	r.Seq = c.seq
	c.mutex.Unlock()
	c.reqHeader = header
	return nil
}

func (c *serverCodec) ReadRequestBody(x interface{}) error {
	if x == nil {
		return nil
	}
	request, ok := x.(proto.Message)
	if !ok {
		return fmt.Errorf(
			"protorpc.ServerCodec.ReadRequestBody: %T does not implement proto.Message",
			x,
		)
	}

	// checksum
	if crc32.ChecksumIEEE(c.reqHeader.Body) != c.reqHeader.Checksum {
		return fmt.Errorf("protorpc.readRequestBody: unexpected checksum.")
	}

	// decode the compressed data
	pbRequest, err := snappy.Decode(nil, c.reqHeader.Body)
	if err != nil {
		return err
	}
	// check wire header: rawMsgLen
	if uint32(len(pbRequest)) != c.reqHeader.RawRequestLen {
		return fmt.Errorf("protorpc.readRequestBody: Unexcpeted header.RawRequestLen.")
	}

	// Unmarshal to proto message
	if request != nil {
		err = proto.Unmarshal(pbRequest, request)
		if err != nil {
			return err
		}
	}
	c.reqHeader = wire.RequestHeader{}
	return nil
}

// A value sent as a placeholder for the server's response value when the server
// receives an invalid request. It is never decoded by the client since the Response
// contains an error when it is used.
var invalidRequest = struct{}{}

func (c *serverCodec) WriteResponse(r *rpc.Response, x interface{}) error {
	err := c.readyQueue()
	if err != nil {
		return err
	}
	var response proto.Message
	if x != nil {
		var ok bool
		if response, ok = x.(proto.Message); !ok {
			if _, ok = x.(struct{}); !ok {
				c.mutex.Lock()
				delete(c.pending, r.Seq)
				c.mutex.Unlock()
				return fmt.Errorf(
					"protorpc.ServerCodec.WriteResponse: %T does not implement proto.Message",
					x,
				)
			}
		}
	}

	c.mutex.Lock()
	attr, ok := c.pending[r.Seq]
	if !ok {
		c.mutex.Unlock()
		return errors.New("protorpc: invalid sequence number in response")
	}
	delete(c.pending, r.Seq)
	c.mutex.Unlock()

	err = c.RBwriteResponse(attr, r.Error, response)
	if err != nil {
		return err
	}

	return nil
}

func (c *serverCodec) RBwriteResponse(attr requestAttr, serr string, response proto.Message) (err error) {
	// clean response if error
	if serr != "" {
		response = nil
	}
	err = c.readyQueue()
	if err != nil {
		return err
	}
	// marshal response
	pbResponse := []byte{}
	if response != nil {
		pbResponse, err = proto.Marshal(response)
		if err != nil {
			return err
		}
	}

	// compress serialized proto data
	compressedPbResponse := snappy.Encode(nil, pbResponse)

	// generate header
	header := &wire.ResponseHeader{
		Id:                          attr.Id,
		Error:                       serr,
		RawResponseLen:              uint32(len(pbResponse)),
		SnappyCompressedResponseLen: uint32(len(compressedPbResponse)),
		Checksum:                    crc32.ChecksumIEEE(compressedPbResponse),
		Body:                        compressedPbResponse,
	}

	// check header size
	pbHeader, err := proto.Marshal(header)
	if err != err {
		return
	}

	err = c.AmqpChannel.Publish(
		c.ExchangeName, // exchange
		attr.ReplyTo,   // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			Body:            pbHeader,
		})
	if err != nil {
		log.Fatal("Publish error: ", err)
		return
	}
	return nil
}

func (c *serverCodec) Close() error {
	var err error
	err = c.AmqpChannel.Close()
	if err == nil {
		err = c.AmqpConnect.Close()
	}

	return err
}

// ServeConn runs the Protobuf-RPC server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
// The caller typically invokes ServeConn in a go statement.
func ServeConn(uri string, exchangeName string, queueName string) {
	rpc.ServeCodec(NewServerCodec(uri, exchangeName, queueName))
}
