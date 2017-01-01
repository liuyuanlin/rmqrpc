// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proto3_proto

import (
	"bytes"
	"encoding/gob"
	"log"
	"os"
	"reflect"
	"testing"
	"time"
)

type tEchoService struct {
	private int
}

func (p *tEchoService) Echo(in *Message, out *Message) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		return err
	}
	if err := gob.NewDecoder(bytes.NewBuffer(buf.Bytes())).Decode(out); err != nil {
		return err
	}
	return nil
}

func TestMain(m *testing.M) {
	go func() {
		if err := StartEchoServiceServer("amqp://guest:guest@localhost:5672/", "", "rpc_queue", new(tEchoService)); err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(time.Second * 3) // wait for start the server
	os.Exit(m.Run())
}

func TestEchoService(t *testing.T) {
	c := NewEchoServiceClient("amqp://guest:guest@localhost:5672/", "", "rpc_queue")

	defer c.Close()

	in := Message{
		Name:         "rmqrpc",
		Hilarity:     Message_PUNS,
		HeightInCm:   13,
		Data:         []byte("bin data"),
		ResultCount:  2<<35 + 1,
		TrueScotsman: true,
		Score:        3.14,
		Key:          []uint64{1, 1001},
		Nested:       &Nested{Bunny: "{{Bunny}}"},
		Terrain: map[string]*Nested{
			"A": &Nested{Bunny: "{{A}}"},
			"B": &Nested{Bunny: "{{B}}"},
		},
	}

	out, err := c.Echo(&in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&in, out) {
		t.Fatalf("not euqal, got = %v\n", &out)
	}
}
