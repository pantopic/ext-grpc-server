package wazero_grpc_server

import (
	"context"

	"google.golang.org/grpc"

	"github.com/pantopic/wazero-pool"
)

type handlerFactory func(context.Context, wazeropool.Instance, *meta, string) grpc.ClientStream

type ContextCopy = func(dst, src context.Context) context.Context
