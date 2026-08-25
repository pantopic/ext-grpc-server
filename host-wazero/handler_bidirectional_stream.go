package wazero_grpc_server

import (
	"context"
	"io"
	"log"
	"log/slog"

	"github.com/tetratelabs/wazero/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/pantopic/wazero-pool"
)

func handlerFactoryBidirectionalStream(ctx context.Context, pool wazeropool.Instance, meta *meta, method string) grpc.ClientStream {
	s := &handlerBidirectionalStream{
		ctx:    ctx,
		data:   make(chan resp),
		meta:   meta,
		method: method,
		pool:   pool,
	}
	s.ctx = context.WithValue(s.ctx, ctxKeyMeta, meta)
	s.ctx = context.WithValue(s.ctx, ctxKeySend, s.send)
	return s
}

type handlerBidirectionalStream struct {
	ctx    context.Context
	data   chan resp
	meta   *meta
	method string
	open   bool
	pool   wazeropool.Instance
}

func (h *handlerBidirectionalStream) Header() (md metadata.MD, err error) {
	return
}

func (h *handlerBidirectionalStream) Trailer() (md metadata.MD) {
	return
}

func (h *handlerBidirectionalStream) CloseSend() (err error) {
	h.pool.Run(func(mod api.Module) {
		setMethod(mod, h.meta, []byte(h.method))
		setErrCode(mod, h.meta, codes.OK)
		fn := "__grpc_server_bidirectional_close"
		_, err = mod.ExportedFunction(fn).Call(h.ctx)
		if err != nil {
			slog.Info(fn, `err`, err)
			return
		}
	})
	return
}

func (h *handlerBidirectionalStream) Context() context.Context {
	return h.ctx
}

func (h *handlerBidirectionalStream) SendMsg(m any) (err error) {
	msg, err := proto.Marshal(m.(proto.Message))
	if err != nil {
		panic(`Unable to marshal message in SendMsg: ` + err.Error())
	}
	h.pool.Run(func(mod api.Module) {
		setMethod(mod, h.meta, []byte(h.method))
		setErrCode(mod, h.meta, codes.OK)
		if !h.open {
			h.open = true
			fn := "__grpc_server_bidirectional_open"
			_, err = mod.ExportedFunction(fn).Call(h.ctx)
			if err != nil {
				slog.Info(fn, `err`, err)
				return
			}
		}
		if err = setMsg(mod, h.meta, msg); err != nil {
			return
		}
		fn := "__grpc_server_bidirectional_recv"
		_, err = mod.ExportedFunction(fn).Call(h.ctx)
		if err != nil {
			slog.Info(fn, `err`, err)
			return
		}
	})
	return
}

func (h *handlerBidirectionalStream) send(ctx context.Context, msg []byte, err error) {
	select {
	case h.data <- resp{append([]byte{}, msg...), err}:
	case <-h.ctx.Done():
		return
	}
}

func (h *handlerBidirectionalStream) RecvMsg(m any) (err error) {
	select {
	case d, ok := <-h.data:
		if !ok {
			return io.EOF
		}
		if d.err != nil {
			log.Printf(`data err %v`, d.err)
			return &Error{
				msg: d.err.Error(),
			}
		}
		err = proto.Unmarshal(d.data, m.(proto.Message))
		if err != nil {
			slog.Info(`RecvMsg`, `err`, err, `data`, d.data)
			log.Fatalf(`Unable to unmarshal message in RecvMsg: %v`, err)
		}
	case <-h.ctx.Done():
		return io.EOF
	}
	return
}
