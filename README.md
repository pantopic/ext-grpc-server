# Wazero gRPC Server

A [wazero](https://pkg.go.dev/github.com/tetratelabs/wazero) host module, ABI and guest SDK enabling WASI modules to register as [gRPC](https://grpc.io/) services.

## Host Module

[![Go Reference](https://godoc.org/github.com/pantopic/ext-grpc-server/host?status.svg)](https://godoc.org/github.com/pantopic/ext-grpc-server/host)
[![Go Report Card](https://goreportcard.com/badge/github.com/pantopic/ext-grpc-server/host)](https://goreportcard.com/report/github.com/pantopic/ext-grpc-server/host)
[![Go Coverage](https://github.com/pantopic/ext-grpc-server/wiki/host/coverage.svg)](https://raw.githack.com/wiki/pantopic/ext-grpc-server/host/coverage.html)

First register the host module with the runtime

```go
import (
	"context"
	_ "embed"
	"net"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"google.golang.org/grpc"

	"github.com/pantopic/wazero-pool"

	"github.com/pantopic/ext-grpc-server/host-wazero"
)

//go:embed test\.wasm
var wasm []byte

func main() {
	ctx := context.Background()
	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig())
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	hostModule := wazero_grpc_server.New()
	hostModule.Register(ctx, runtime)

	grpcServer := grpc.NewServer()
	pool, err := wazeropool.New(ctx, runtime, wasm)
	if err != nil {
		panic(err)
	}
	hostModule.RegisterServices(ctx, grpcServer, pool)

	grpcListener, _ := net.Listen(`tcp`, `:8000`)
	grpcServer.Serve(grpcListener)

	// ...
}
```

## Guest SDK (Go)

[![Go Reference](https://godoc.org/github.com/pantopic/ext-grpc-server/grpc-server-go?status.svg)](https://godoc.org/github.com/pantopic/ext-grpc-server/grpc-server-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/pantopic/ext-grpc-server/grpc-server-go)](https://goreportcard.com/report/github.com/pantopic/ext-grpc-server/grpc-server-go)

Then you can import the guest SDK into your WASI module to export your gRPC service description at runtime and receive gRPC requests in WASM.

```go
package main

import (
	"github.com/pantopic/ext-grpc-server/sdk-go"
	"github.com/pantopic/ext-grpc-server/sdk-go/codes"

	"github.com/pantopic/ext-grpc-server/test-lite/pb"
)

func init() {
	grpc_server.Init()
	grpc_server.NewService(`test.TestService`).
		Unary(`Test`, test).
}

func main() {}

var (
	req  = new(pb.TestRequest)
	resp = new(pb.TestResponse)
)

func test(b []byte) (err error) {
	req.Reset() 
	if err = req.Unmarshal(b); err != nil {
		err = status.New(codes.InvalidArgument, err.Error()).Err()
		return
	}
	resp.Reset()
	resp.Bar = req.Foo
	return grpc_server.Send(resp.Marshal(nil))
}

```

The [guest SDK](https://pkg.go.dev/github.com/pantopic/ext-grpc-server/grpc-server-go) has no dependencies outside the Go std lib.
The guest SDK is serialization agnostic in order to provide users with more control over performance, compile time and binary size.

See examples for protobuf message serialization options:

- [test-easy](/test-easy) - `easyproto` manually-generated (`100kb` binary, `3s` build time)
- [test-lite](/test-lite) - `protobuf-go-lite` auto-generated (`100kb` binary, `3s` build time, recommended)

These options and others can be used for protobuf serialization in WASM but some standard approaches to protobuf
serialization like `protoc-gen-go` require reflection which tinygo does not support.

## Performance

Setting `limit` explicitly is recommended unless you are already strictly limiting the concurrency of the calling code
in other ways.

```go
> make bench
BenchmarkHostModule/linear/testWasmEasy-16                 13602             78630 ns/op
BenchmarkHostModule/linear/testWasmLite-16                 14311             76914 ns/op
BenchmarkHostModule/linear/testWasmEasyProd-16             13910             78315 ns/op
BenchmarkHostModule/linear/testWasmLiteProd-16             14396             76460 ns/op
BenchmarkHostModule/parallel-0/testWasmEasyProd-16         85554             12037 ns/op
BenchmarkHostModule/parallel-0/testWasmLiteProd-16         94183             11982 ns/op
BenchmarkHostModule/parallel-2/testWasmEasyProd-16        148317              8109 ns/op
BenchmarkHostModule/parallel-2/testWasmLiteProd-16        129385              8135 ns/op
BenchmarkHostModule/parallel-4/testWasmEasyProd-16        187816              6191 ns/op
BenchmarkHostModule/parallel-4/testWasmLiteProd-16        199989              6017 ns/op
BenchmarkHostModule/parallel-8/testWasmEasyProd-16        231013              5185 ns/op
BenchmarkHostModule/parallel-8/testWasmLiteProd-16        232335              5172 ns/op
BenchmarkHostModule/parallel-16/testWasmEasyProd-16       248992              4871 ns/op
BenchmarkHostModule/parallel-16/testWasmLiteProd-16       247765              4928 ns/op
```

## Roadmap

This project is in alpha. Breaking API changes should be expected until Beta.

- `v0.0.x` - Alpha
  - [ ] Server streaming support
  - [ ] Client streaming support
  - [ ] Bidirectional streaming support
  - [ ] Asynchronous unary support
  - [ ] Stabilize API
- `v0.x.x` - Beta
  - [ ] Finalize API
  - [ ] Test in production
- `v1.x.x` - General Availability
  - [ ] Proven long term stability in production
