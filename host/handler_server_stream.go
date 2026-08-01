package wazero_grpc_server

import (
	"context"
	"io"

	"github.com/tetratelabs/wazero/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/pantopic/wazero-pool"
)

func handlerFactoryServerStream(ctx context.Context, pool wazeropool.Instance, meta *meta, method string) grpc.ClientStream {
	s := &handlerServerStream{
		ctx:    ctx,
		data:   make(chan resp, 64),
		meta:   meta,
		method: method,
		pool:   pool,
	}
	s.ctx = context.WithValue(s.ctx, ctxKeyMeta, meta)
	s.ctx = context.WithValue(s.ctx, ctxKeySend, s.send)
	return s
}

type handlerServerStream struct {
	ctx    context.Context
	data   chan resp
	meta   *meta
	method string
	pool   wazeropool.Instance
}

func (h *handlerServerStream) Header() (md metadata.MD, err error) {
	return
}

func (h *handlerServerStream) Trailer() (md metadata.MD) {
	return
}

func (h *handlerServerStream) CloseSend() (err error) {
	h.pool.Run(func(mod api.Module) {
		setMethod(mod, h.meta, []byte(h.method))
		mod.ExportedFunction("__grpc_server_server_stream_close").Call(h.ctx)
	})
	return
}

func (h *handlerServerStream) Context() context.Context {
	return h.ctx
}

func (h *handlerServerStream) send(msg []byte, err error) {
	select {
	case h.data <- resp{append([]byte{}, msg...), err}:
	case <-h.ctx.Done():
		return
	}
}

func (h *handlerServerStream) SendMsg(m any) (err error) {
	msg, err := proto.Marshal(m.(proto.Message))
	if err != nil {
		panic(err)
	}
	h.pool.Run(func(mod api.Module) {
		setMethod(mod, h.meta, []byte(h.method))
		setMsg(mod, h.meta, msg)
		mod.ExportedFunction("__grpc_server_server_stream_open").Call(h.ctx)
	})
	return
}

func (h *handlerServerStream) RecvMsg(m any) (err error) {
	select {
	case d, ok := <-h.data:
		if !ok {
			return io.EOF
		}
		if d.err != nil {
			return &Error{
				msg: d.err.Error(),
			}
		}
		err = proto.Unmarshal(d.data, m.(proto.Message))
	case <-h.ctx.Done():
		return io.EOF
	}
	return
}
