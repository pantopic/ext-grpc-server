package wazero_grpc_server

import (
	"context"
	"io"

	"github.com/tetratelabs/wazero/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/pantopic/wazero-pool"
)

func handlerFactoryUnary(ctx context.Context, pool wazeropool.Instance, meta *meta, method string) grpc.ClientStream {
	s := &handlerUnary{
		ctx:    ctx,
		meta:   meta,
		method: method,
		pool:   pool,
		data:   make(chan resp),
	}
	s.ctx = context.WithValue(s.ctx, ctxKeyMeta, meta)
	s.ctx = context.WithValue(s.ctx, ctxKeySend, s.send)
	return s
}

type handlerUnary struct {
	ctx    context.Context
	meta   *meta
	method string
	pool   wazeropool.Instance
	data   chan resp
}

type resp struct {
	data []byte
	err  error
}

func (h *handlerUnary) Header() (md metadata.MD, err error) {
	return
}

func (h *handlerUnary) Trailer() (md metadata.MD) {
	return
}

func (h *handlerUnary) CloseSend() (err error) {
	return
}

func (h *handlerUnary) Context() context.Context {
	return h.ctx
}

func (h *handlerUnary) send(ctx context.Context, msg []byte, err error) {
	select {
	case h.data <- resp{msg, err}:
	case <-h.ctx.Done():
		return
	}
}

func (h *handlerUnary) SendMsg(m any) (err error) {
	data, err := proto.Marshal(m.(proto.Message))
	if err != nil {
		panic(err)
	}
	ctx := h.ctx
	h.pool.Run(func(mod api.Module) {
		setMethod(mod, h.meta, []byte(h.method))
		if err = setMsg(mod, h.meta, data); err != nil {
			return
		}
		setErrCode(mod, h.meta, codes.OK)
		if _, err = mod.ExportedFunction("__grpc_server_unary").Call(ctx); err != nil {
			return
		}
	})
	return
}

func (h *handlerUnary) RecvMsg(m any) (err error) {
	select {
	case resp, ok := <-h.data:
		if !ok {
			return io.EOF
		}
		close(h.data)
		if resp.err != nil {
			return resp.err
		}
		err = proto.Unmarshal(resp.data, m.(proto.Message))
	case <-h.ctx.Done():
		return io.EOF
	}
	return
}
