// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate protoc --plugin=protoc-gen-go=../../protoc-gen-go/protoc-gen-go --go_out=plugins=rmqrpc:. echo.proto arith.proto

package service
