// Borrowed heavily from mwitkow/grpc-proxy (Apache 2.0)
// See https://github.com/mwitkow/grpc-proxy/blob/master/proxy/handler.go

package wazero_grpc_server

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/pantopic/wazero-pool"
)

type grpcHandler struct {
	pool wazeropool.Instance
	meta *meta
	ctx  context.Context
	init []ContextCopy
}

func (h *grpcHandler) handle(f handlerFactory) func(srv any, serverStream grpc.ServerStream) error {
	return func(srv any, serverStream grpc.ServerStream) error {
		fullMethodName, ok := grpc.MethodFromServerStream(serverStream)
		if !ok {
			return status.Errorf(codes.Internal, "lowLevelServerStream not exists in context")
		}
		ctx, cancel := context.WithCancel(serverStream.Context())
		defer cancel()
		for _, cc := range h.init {
			ctx = cc(ctx, h.ctx)
		}
		clientStream := f(ctx, h.pool, h.meta, fullMethodName)
		errChanInbound := h.forwardInbound(serverStream, clientStream)
		errChanOutbound := h.forwardOutbound(clientStream, serverStream)
		for range 2 {
			select {
			case errInbound := <-errChanInbound:
				if errInbound == io.EOF {
					clientStream.CloseSend()
				} else {
					return errInbound
				}
			case errOutbound := <-errChanOutbound:
				serverStream.SetTrailer(clientStream.Trailer())
				if errOutbound != io.EOF {
					return errOutbound
				}
				clientStream.CloseSend()
				return nil
			}
		}
		return status.Errorf(codes.Internal, "gRPC proxying should never reach this stage.")
	}
}

func (h *grpcHandler) forwardInbound(src grpc.ServerStream, dst grpc.ClientStream) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &emptypb.Empty{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err
				break
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
	}()
	return ret
}

func (h *grpcHandler) forwardOutbound(src grpc.ClientStream, dst grpc.ServerStream) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &emptypb.Empty{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err
				break
			}
			if i == 0 {
				md, err := src.Header()
				if err != nil {
					ret <- err
					break
				}
				if err := dst.SendHeader(md); err != nil {
					ret <- err
					break
				}
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
	}()
	return ret
}
