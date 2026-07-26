package wazero_grpc_server

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/pantopic/wazero-atomic/host"
	"github.com/pantopic/wazero-pool"

	"github.com/pantopic/wazero-grpc-server/host/pb"
)

//go:embed test-easy\.wasm
var testWasmEasy []byte

//go:embed test-easy\.prod\.wasm
var testWasmEasyProd []byte

//go:embed test-lite\.wasm
var testWasmLite []byte

//go:embed test-lite\.prod\.wasm
var testWasmLiteProd []byte

//go:embed test-zig\.wasm
var testWasmZig []byte

func TestHostModule(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig())
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	var hostModule *hostModule
	t.Run(`register`, func(t *testing.T) {
		hostModule = New()
		hostModule.Register(ctx, r)
	})
	modAtomic := wazero_atomic.New()
	modAtomic.Register(ctx, r)

	port := 9000
	for _, tc := range []struct {
		name         string
		wasm         []byte
		bufCapMethod uint32
		bufCapMsg    uint32
	}{
		{`testWasmEasy`, testWasmEasy, 256, 1 * 1024 * 1024},
		{`testWasmLite`, testWasmLite, 128, 1.5 * 1024 * 1024},
		{`testWasmEasyProd`, testWasmEasyProd, 256, 1 * 1024 * 1024},
		{`testWasmLiteProd`, testWasmLiteProd, 128, 1.5 * 1024 * 1024},
		{`testWasmZig`, testWasmZig, 128, 1.5 * 1024 * 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := wazero.NewModuleConfig().WithStdout(os.Stdout)
			pool, err := wazeropool.New(ctx, r, tc.wasm,
				wazeropool.WithModuleConfig(cfg))
			if err != nil {
				t.Fatalf(`%v`, err)
			}
			pool.Run(func(mod api.Module) {
				ctx, err = hostModule.InitContext(ctx, mod)
				if err != nil {
					t.Fatalf(`%v`, err)
				}
				ctx, err = modAtomic.InitContext(ctx, mod)
				if err != nil {
					t.Fatalf(`%v`, err)
				}
			})
			ctxCopy := []ContextCopy{modAtomic.ContextCopy}
			pool.Run(func(mod api.Module) {
				meta := get[*meta](ctx, ctxKeyMeta)
				if readUint32(mod, meta.ptrMethodCap) != tc.bufCapMethod {
					t.Errorf("incorrect maximum method length: %#v", meta)
				}
				if readUint32(mod, meta.ptrMsgCap) != tc.bufCapMsg {
					t.Errorf("incorrect maximum method length: %#v", meta)
				}
			})
			port++
			addr := fmt.Sprintf(`:%d`, port)
			lis, err := net.Listen(`tcp`, addr)
			if err != nil {
				t.Fatalf(`%v`, err)
			}
			hostModule.ServerStart(ctx, lis, "", "", pool, ctxCopy...)
			// defer hostModule.ServerStop(ctx, lis)
			conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatalf(`%v`, err)
			}
			client := pb.NewTestServiceClient(conn)
			t.Run(`unary`, func(t *testing.T) {
				t.Run(`basic`, func(t *testing.T) {
					req := &pb.TestRequest{
						Foo: 1,
					}
					res, err := client.Test(ctx, req)
					if err != nil {
						t.Fatalf(`%v`, err)
					}
					if res.Bar != req.Foo {
						t.Fatalf(`Incorrect response value in test response: %d`, res.Bar)
					}
					res2, err := client.Retest(ctx, &pb.RetestRequest{
						Bar: 11,
					})
					if err != nil {
						t.Fatalf(`%v`, err)
					}
					if res2.Foo != 11 {
						t.Fatalf(`Incorrect response value in retest response: %d`, res2.Foo)
					}
				})
				t.Run(`bytes`, func(t *testing.T) {
					req := &pb.TestBytesRequest{
						Key: []byte(`test-key`),
						Val: []byte(`test-value`),
					}
					res, err := client.TestBytes(ctx, req)
					if err != nil {
						t.Fatalf(`%v`, err)
					}
					if res.Code != 1 {
						t.Fatalf(`Incorrect response code in test response: %d`, res.Code)
					}
					if !bytes.Equal(res.Data, []byte(`ACK`)) {
						t.Fatalf(`Incorrect response data in test response: %s`, res.Data)
					}
				})
			})
			t.Run(`client-stream`, func(t *testing.T) {
				cs, err := client.ClientStream(ctx)
				if err != nil {
					t.Fatalf(`%v`, err)
				}
				for range 50 {
					if err = cs.Send(&pb.ClientStreamRequest{
						Foo2: 2,
					}); err != nil {
						t.Fatalf(`%v`, err)
					}
				}
				res, err := cs.CloseAndRecv()
				if err != nil {
					t.Fatalf(`%v`, err)
				}
				if res.Bar2 != 100 {
					t.Fatalf(`Incorrect response value in ClientStream response: %d`, res.Bar2)
				}
			})
			t.Run(`server-stream`, func(t *testing.T) {
				ss, err := client.ServerStream(ctx, &pb.ServerStreamRequest{
					Foo3: 10,
				})
				if err != nil {
					t.Fatalf(`%v`, err)
				}
				var i uint64
				for i = range 10 {
					res, err := ss.Recv()
					if err != nil {
						t.Fatalf(`%v`, err)
					}
					if res.Bar3 != i {
						t.Fatalf(`Wrong number %v != %v`, res.Bar3, i)
					}
				}
				err = ss.CloseSend()
				if err != nil {
					t.Fatalf(`%v`, err)
				}
				if i != 9 {
					t.Fatalf(`Expected 9 got %v`, i)
				}
			})
			t.Run(`bidirectional-stream`, func(t *testing.T) {
				s, err := client.BidirectionalStream(ctx)
				if err != nil {
					t.Fatalf(`%v`, err)
				}
				go func() {
					for i := range 10 {
						s.Send(&pb.BidirectionalStreamRequest{
							Foo4: uint64(i),
						})
					}
				}()
				var n uint64
				var i uint64
				for i = range 10 {
					res, err := s.Recv()
					if err != nil {
						t.Fatalf(`%v`, err)
					}
					n += i
					if res.Bar4 != n {
						t.Fatalf(`Wrong number %v != %v`, res.Bar4, n)
					}
				}
				err = s.CloseSend()
				if err != nil {
					t.Fatalf(`%v`, err)
				}
				if i != 9 {
					t.Fatalf(`Expected 9 got %v`, i)
				}
			})
			t.Run(`http`, func(t *testing.T) {
				c := &http.Client{Timeout: time.Second}
				t.Run(`GET`, func(t *testing.T) {
					t.Run(`200`, func(t *testing.T) {
						res, err := c.Get(fmt.Sprintf(`http://%s/hello`, addr))
						if err != nil {
							t.Fatalf(`%v`, err)
						}
						defer res.Body.Close()
						if res.StatusCode != 200 {
							t.Fatalf(`Incorrect status code: %d`, res.StatusCode)
						}
						b, err := io.ReadAll(res.Body)
						if err != nil {
							t.Fatalf(`%v`, err)
						}
						if string(b) != "world" {
							t.Fatalf(`Incorrect response body: %s`, string(b))
						}
					})
					t.Run(`410`, func(t *testing.T) {
						res, err := c.Get(fmt.Sprintf(`http://%s/goodbye`, addr))
						if err != nil {
							t.Fatalf(`%v`, err)
						}
						defer res.Body.Close()
						if res.StatusCode != 410 {
							t.Fatalf(`Incorrect status code: %d`, res.StatusCode)
						}
						b, err := io.ReadAll(res.Body)
						if err != nil {
							t.Fatalf(`%v`, err)
						}
						if string(b) != "world" {
							t.Fatalf(`Incorrect response body: %s`, string(b))
						}
					})
				})
			})
		})
	}
}

func BenchmarkHostModule(b *testing.B) {
	var ctx = context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig())
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	hostModule := New()
	hostModule.Register(ctx, r)
	modAtomic := wazero_atomic.New()
	modAtomic.Register(ctx, r)
	b.Run(`linear`, func(b *testing.B) {
		for _, tc := range []struct {
			name string
			wasm []byte
		}{
			{`testWasmEasy`, testWasmEasy},
			{`testWasmLite`, testWasmLite},
			{`testWasmEasyProd`, testWasmEasyProd},
			{`testWasmLiteProd`, testWasmLiteProd},
		} {
			s := grpc.NewServer()
			cfg := wazero.NewModuleConfig().WithStdout(os.Stdout)
			pool, err := wazeropool.New(ctx, r, tc.wasm, wazeropool.WithModuleConfig(cfg))
			if err != nil {
				b.Fatalf(`%v`, err)
			}
			addr := `:9001`
			pool.Run(func(mod api.Module) {
				ctx, err = hostModule.InitContext(ctx, mod)
			})
			err = hostModule.RegisterServices(ctx, s, pool)
			if err != nil {
				b.Fatalf(`%v`, err)
			}
			lis, _ := net.Listen(`tcp`, addr)
			go func() {
				if err := s.Serve(lis); err != nil {
					panic(err)
				}
			}()
			conn, _ := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			client := pb.NewTestServiceClient(conn)
			req := &pb.TestRequest{Foo: 20}
			var res *pb.TestResponse
			b.Run(tc.name, func(b *testing.B) {
				for b.Loop() {
					res, _ = client.Test(ctx, req)
				}
			})
			if res.Bar != 20 {
				b.Fatalf(`Nope`)
			}
			s.Stop()
		}
	})
	for _, n := range []int{0, 2, 4, 8, 16} {
		b.Run(fmt.Sprintf(`parallel-%d`, n), func(b *testing.B) {
			for _, tc := range []struct {
				name string
				wasm []byte
			}{
				{`testWasmEasyProd`, testWasmEasyProd},
				{`testWasmLiteProd`, testWasmLiteProd},
			} {
				s := grpc.NewServer()
				cfg := wazero.NewModuleConfig().WithStdout(os.Stdout)
				pool, err := wazeropool.New(ctx, r, tc.wasm,
					wazeropool.WithModuleConfig(cfg),
					wazeropool.WithLimit(n))
				if err != nil {
					b.Fatalf(`%v`, err)
				}
				addr := `:9001`
				pool.Run(func(mod api.Module) {
					ctx, err = hostModule.InitContext(ctx, mod)
				})
				err = hostModule.RegisterServices(ctx, s, pool)
				if err != nil {
					b.Fatalf(`%v`, err)
				}
				lis, _ := net.Listen(`tcp`, addr)
				go func() {
					if err := s.Serve(lis); err != nil {
						panic(err)
					}
				}()
				conn, _ := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
				client := pb.NewTestServiceClient(conn)
				req := &pb.TestRequest{Foo: 20}
				var res *pb.TestResponse
				b.Run(tc.name, func(b *testing.B) {
					b.SetParallelism(n)
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							res, _ = client.Test(ctx, req)
						}
					})
				})
				if res.Bar != 20 {
					b.Fatalf(`Nope`)
				}
				s.Stop()
			}
		})
	}
}
