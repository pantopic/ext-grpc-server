package wazero_grpc_server

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/tetratelabs/wazero/api"
	"google.golang.org/grpc"

	"github.com/pantopic/wazero-pool"
)

type httpHandler struct {
	ctx        context.Context
	grpcServer *grpc.Server
	pool       wazeropool.Instance
}

func NewHttpHandler(ctx context.Context, grpcServer *grpc.Server, pool wazeropool.Instance) *httpHandler {
	return &httpHandler{
		ctx:        ctx,
		grpcServer: grpcServer,
		pool:       pool,
	}
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor == 2 && strings.HasPrefix(
		r.Header.Get("Content-Type"), "application/grpc") {
		h.grpcServer.ServeHTTP(w, r)
		return
	}
	h.pool.Run(func(mod api.Module) {
		fn := mod.ExportedFunction(`__grpc_server_http`)
		if fn == nil {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		meta := get[*meta](h.ctx, ctxKeyMeta)
		body, _ := io.ReadAll(r.Body)
		setMethod(mod, meta, []byte(r.Method+" "+r.URL.Path))
		setMsg(mod, meta, body)
		setErrCode(mod, meta, 0)
		fn.Call(h.ctx)
		if code := int(getErrCode(mod, meta)); code != 0 {
			w.WriteHeader(code)
		}
		w.Write(getMsgCopy(mod, meta))
	})
}
