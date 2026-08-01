package wazero_grpc_server

import (
	"context"
	"io"
	"log"

	"github.com/tetratelabs/wazero/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/pantopic/wazero-pool"
)

func handlerFactoryClientStream(ctx context.Context, pool wazeropool.Instance, meta *meta, method string) grpc.ClientStream {
	s := &handlerClientStream{
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

type handlerClientStream struct {
	ctx    context.Context
	pool   wazeropool.Instance
	meta   *meta
	method string
	data   chan resp
	init   bool
}

func (h *handlerClientStream) Header() (md metadata.MD, err error) {
	return
}

func (h *handlerClientStream) Trailer() (md metadata.MD) {
	return
}

func (h *handlerClientStream) CloseSend() (err error) {
	h.pool.Run(func(mod api.Module) {
		setMethod(mod, h.meta, []byte(h.method))
		mod.ExportedFunction("__grpc_server_client_stream_close").Call(h.ctx)
	})
	return
}

func (h *handlerClientStream) Context() context.Context {
	return h.ctx
}

func (h *handlerClientStream) send(msg []byte, err error) {
	select {
	case h.data <- resp{append([]byte{}, msg...), err}:
	case <-h.ctx.Done():
		return
	}
}

func (h *handlerClientStream) SendMsg(m any) (err error) {
	msg, err := proto.Marshal(m.(proto.Message))
	if err != nil {
		panic(err)
	}
	h.pool.Run(func(mod api.Module) {
		// Special case for first message
		setMethod(mod, h.meta, []byte(h.method))
		setErrCode(mod, h.meta, codes.OK)
		if !h.init {
			h.init = true
			_, err := mod.ExportedFunction("__grpc_server_client_stream_open").Call(h.ctx)
			if err != nil {
				log.Println(err)
				return
			}
		}
		setMsg(mod, h.meta, msg)
		_, err := mod.ExportedFunction("__grpc_server_client_stream_recv").Call(h.ctx)
		if err != nil {
			log.Println(err)
			return
		}
	})
	return
}

func (h *handlerClientStream) RecvMsg(m any) (err error) {
	select {
	case r, ok := <-h.data:
		if !ok {
			return io.EOF
		}
		close(h.data)
		if r.err != nil {
			return r.err
		}
		err = proto.Unmarshal(r.data, m.(proto.Message))
	case <-h.ctx.Done():
		return io.EOF
	}
	return
}
