// Copyright 2013 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rmqrpc

import (
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

type clientCodec struct {
	Uri          string
	ExchangeName string
	QueueName    string
	ReplyTo      string
	AmqpConnect  *amqp.Connection
	AmqpChannel  *amqp.Channel
	AmqpQueue    amqp.Queue
	AmqMsgs      <-chan amqp.Delivery

	// temporary work space
	respHeader wire.ResponseHeader

	// Protobuf-RPC responses include the request id but not the request method.
	// Package rpc expects both.
	// We save the request method in pending when sending a request
	// and then look it up by request ID when filling out the rpc Response.
	mutex   sync.Mutex        // protects pending
	pending map[uint64]string // map request id to method name
}

// NewClientCodec returns a new rpc.ClientCodec using Protobuf-RPC on conn.
func NewClientCodec(uri string, exchangeName string, queueName string) rpc.ClientCodec {
	return &clientCodec{
		Uri:          uri,
		ExchangeName: exchangeName,
		QueueName:    queueName,
		pending:      make(map[uint64]string),
	}
}

func (c *clientCodec) readyQueue() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	var err error
	if c.AmqpConnect == nil {
		log.Println("start connect broker")
		c.AmqpConnect, err = amqp.Dial(c.Uri)
		if err != nil {
			log.Println("amqp dial error:%v", err)
			return err
		}
	}

	if c.AmqpChannel == nil {
		log.Println("connect broker success")
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
	}

	if c.ReplyTo == "" {
		log.Println("tempQueue  create  gdfgsfdfd")
		tempQueue, err := c.AmqpChannel.QueueDeclare("", false, false, false, false, nil)
		if err != nil {
			log.Println("amqp Callback Queue Declare error:%v", err)
			return err
		}
		c.AmqMsgs, err = c.AmqpChannel.Consume(
			tempQueue.Name, // queue
			"",             // consumer
			true,           // auto-ack
			false,          // exclusive
			false,          // no-local
			false,          // no-wait
			nil,            // args
		)
		if err != nil {
			log.Println("Consume fail :", err)
			return err
		}

		c.ReplyTo = tempQueue.Name
		log.Println("tempQueue  name:", c.ReplyTo)
	}

	return nil
}

func (c *clientCodec) WriteRequest(r *rpc.Request, param interface{}) error {
	err := c.readyQueue()
	if err != nil {
		return err
	}
	c.mutex.Lock()
	c.pending[r.Seq] = r.ServiceMethod
	c.mutex.Unlock()

	var request proto.Message
	if param != nil {
		var ok bool
		if request, ok = param.(proto.Message); !ok {
			return fmt.Errorf(
				"protorpc.ClientCodec.WriteRequest: %T does not implement proto.Message",
				param,
			)
		}
	}
	err = c.RBwriteRequest(r.Seq, r.ServiceMethod, request)
	if err != nil {
		return err
	}

	return nil
}

func (c *clientCodec) RBwriteRequest(id uint64, method string, request proto.Message) error {
	err := c.readyQueue()
	if err != nil {
		return err
	}
	// marshal request
	pbRequest := []byte{}
	if request != nil {
		var err error
		pbRequest, err = proto.Marshal(request)
		if err != nil {
			return err
		}
	}

	// compress serialized proto data
	compressedPbRequest := snappy.Encode(nil, pbRequest)

	// generate header
	header := &wire.RequestHeader{
		Id:                         id,
		Method:                     method,
		RawRequestLen:              uint32(len(pbRequest)),
		SnappyCompressedRequestLen: uint32(len(compressedPbRequest)),
		Checksum:                   crc32.ChecksumIEEE(compressedPbRequest),
		Body:                       compressedPbRequest,
	}

	// check header size
	pbHeader, err := proto.Marshal(header)
	if err != err {
		return err
	}

	log.Println("RBwriteRequest : phHeader len = ", len(pbHeader))
	err = c.AmqpChannel.Publish(
		c.ExchangeName,   // exchange
		c.AmqpQueue.Name, // routing key
		false,            // mandatory
		false,            // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			ReplyTo:         c.ReplyTo,
			Body:            pbHeader,
		})
	if err != nil {
		log.Fatal("Publish error: ", err)
		return err
	}

	return nil
}

func (c *clientCodec) ReadResponseHeader(r *rpc.Response) error {
	err := c.readyQueue()
	if err != nil {
		return err
	}
	header := wire.ResponseHeader{}
	msg := <-c.AmqMsgs
	// Marshal Header
	err = proto.Unmarshal(msg.Body, &header)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	r.Seq = header.Id
	r.Error = header.Error
	r.ServiceMethod = c.pending[r.Seq]
	delete(c.pending, r.Seq)
	c.mutex.Unlock()

	c.respHeader = header
	return nil
}

func (c *clientCodec) ReadResponseBody(x interface{}) error {
	var response proto.Message
	if x != nil {
		var ok bool
		response, ok = x.(proto.Message)
		if !ok {
			return fmt.Errorf(
				"protorpc.ClientCodec.ReadResponseBody: %T does not implement proto.Message",
				x,
			)
		}
	}

	compressedPbResponse := c.respHeader.Body

	// checksum
	if crc32.ChecksumIEEE(compressedPbResponse) != c.respHeader.Checksum {
		return fmt.Errorf("protorpc.readResponseBody: unexpected checksum.")
	}

	// decode the compressed data
	pbResponse, err := snappy.Decode(nil, compressedPbResponse)
	if err != nil {
		return err
	}
	// check wire header: rawMsgLen
	if uint32(len(pbResponse)) != c.respHeader.RawResponseLen {
		return fmt.Errorf("protorpc.readResponseBody: Unexcpeted header.RawResponseLen.")
	}

	// Unmarshal to proto message
	if response != nil {
		err = proto.Unmarshal(pbResponse, response)
		if err != nil {
			return err
		}
	}

	c.respHeader = wire.ResponseHeader{}
	return nil
}

// Close closes the underlying connection.
func (c *clientCodec) Close() error {
	var err error
	err = c.AmqpChannel.Close()
	if err == nil {
		err = c.AmqpConnect.Close()
	}

	return err
}

// NewClient returns a new rpc.Client to handle requests to the
// set of services at the other end of the connection.
func NewClient(uri string, exchangeName string, queueName string) *rpc.Client {
	return rpc.NewClientWithCodec(NewClientCodec(uri, exchangeName, queueName))
}

// Dial connects to a Protobuf-RPC server at the specified network address.
func Dial(uri string, exchangeName string, queueName string) (*rpc.Client, error) {

	temConnect, err := amqp.Dial(uri)
	if err != nil {
		log.Println("amqp dial error:%v", err)
		return nil, err
	}
	temConnect.Close()
	return NewClient(uri, exchangeName, queueName), err
}
