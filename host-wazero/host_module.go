package wazero_grpc_server

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/soheilhy/cmux"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/pantopic/wazero-pool"
)

const Name = "pantopic/ext-grpc-server"

var (
	ctxKeyMeta = Name + `/meta`
	ctxKeyNext = Name + `/next`
	ctxKeySend = Name + `/send`
)

type meta struct {
	ptrMethodCap uint32
	ptrMethodLen uint32
	ptrMethod    uint32
	ptrMsgCap    uint32
	ptrMsgLen    uint32
	ptrMsg       uint32
	ptrErrCode   uint32
}

type hostModule struct {
	sync.RWMutex

	module api.Module
	mux    map[net.Listener]cmux.CMux
}

type sendFunc = func(ctx context.Context, msg []byte, err error)

func New(opts ...Option) (h *hostModule) {
	h = &hostModule{
		mux: make(map[net.Listener]cmux.CMux),
	}
	for _, opt := range opts {
		opt(h)
	}
	return
}

func (h *hostModule) Name() string {
	return Name
}

// Register instantiates the host module, making it available to all module instances in this runtime
func (h *hostModule) Register(ctx context.Context, r wazero.Runtime) (err error) {
	builder := r.NewHostModuleBuilder(Name)
	register := func(name string, fn func(ctx context.Context, m api.Module, stack []uint64)) {
		builder = builder.NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(fn), []api.ValueType{api.ValueTypeI64}, nil).Export(name)
	}
	for name, fn := range map[string]any{
		"__grpc_server_send": func(ctx context.Context, msg []byte, err error) {
			get[sendFunc](ctx, ctxKeySend)(ctx, msg, err)
		},
	} {
		switch fn := fn.(type) {
		case func(context.Context, []byte, error):
			register(name, func(ctx context.Context, mod api.Module, stack []uint64) {
				meta := get[*meta](ctx, ctxKeyMeta)
				var data []byte
				if uint32(stack[0]) > 0 {
					buf, _ := mod.Memory().Read(uint32(stack[0]>>32), uint32(stack[0]))
					data = append(data[:0], buf...)
				}
				fn(ctx, data, getError(mod, meta, data))
			})
		default:
			log.Panicf("Method signature implementation missing: %#v", fn)
		}
	}
	h.module, err = builder.Instantiate(ctx)
	return
}

// InitContext retrieves the meta page from the wasm module
func (h *hostModule) InitContext(ctx context.Context, m api.Module) (context.Context, error) {
	fn := m.ExportedFunction(`__grpc_server`)
	if fn == nil {
		return ctx, nil
	}
	stack, err := fn.Call(ctx)
	if err != nil {
		return ctx, err
	}
	meta := &meta{}
	ptr := uint32(stack[0])
	for i, v := range []*uint32{
		&meta.ptrMethodCap,
		&meta.ptrMethodLen,
		&meta.ptrMethod,
		&meta.ptrMsgCap,
		&meta.ptrMsgLen,
		&meta.ptrMsg,
		&meta.ptrErrCode,
	} {
		*v = readUint32(m, ptr+uint32(4*i))
	}
	return context.WithValue(ctx, ctxKeyMeta, meta), nil
}

// ContextCopy populates dst context with the meta page from src context.
func (h *hostModule) ContextCopy(dst, src context.Context) context.Context {
	if v := src.Value(ctxKeyMeta); v != nil {
		dst = context.WithValue(dst, ctxKeyMeta, v.(*meta))
		if v := src.Value(ctxKeyNext); v != nil {
			dst = context.WithValue(dst, ctxKeyNext, v.(chan []byte))
		}
		if v := src.Value(ctxKeySend); v != nil {
			dst = context.WithValue(dst, ctxKeySend, v.(sendFunc))
		}
	}
	return dst
}

// RegisterServices attaches the grpc service(s) to the grpc server
// Called once before server open
func (h *hostModule) RegisterServices(ctx context.Context, s *grpc.Server, pool wazeropool.Instance, ctxCopiers ...ContextCopy) error {
	ctx = wazeropool.ContextSet(ctx, pool)
	ctxCopiers = append(ctxCopiers, wazeropool.ContextCopy)
	meta := get[*meta](ctx, ctxKeyMeta)
	pool.Run(func(mod api.Module) {
		mod.ExportedFunction(`__grpc_server`).Call(ctx)
		// Format: "/package1.ServiceName/u.method1,c.method2/service2.ServiceName/s.method1,b.method2"
		msg := string(getMsg(mod, meta))
		parts := strings.Split(msg, "/")
		for i := 1; i+2 <= len(parts); i += 2 {
			h.registerService(s, pool, meta, parts[i], strings.Split(parts[i+1], ","), ctx, ctxCopiers...)
		}
	})
	return nil
}

func (h *hostModule) ServiceType() string {
	return `grpc`
}

func (h *hostModule) ServerStart(ctx context.Context, lis net.Listener, tlsCrt, tlsKey string, pool wazeropool.Instance, ctxCopy ...ContextCopy) (err error) {
	var opts = []grpc.ServerOption{
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: false,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    2 * time.Hour,
			Timeout: 20 * time.Second,
		}),
		grpc.MaxRecvMsgSize(4 << 30), // 4 GiB
	}
	var grpcServer = grpc.NewServer(opts...)
	err = h.RegisterServices(ctx, grpcServer, pool, ctxCopy...)
	if err != nil {
		return err
	}
	m := cmux.New(lis)
	grpcListener := m.Match(cmux.HTTP2())
	httpListener := m.Match(cmux.Any())
	go func() {
		if err = grpcServer.Serve(grpcListener); err != nil {
			panic(err)
		}
	}()
	httpServer := &http.Server{
		Handler: NewHttpHandler(ctx, grpcServer, pool),
	}
	go func() {
		if tlsCrt != "" && tlsKey != "" {
			if err = httpServer.ServeTLS(httpListener, tlsCrt, tlsKey); err != nil {
				panic(err)
			}
		} else {
			if err = httpServer.Serve(httpListener); err != nil {
				panic(err)
			}
		}
		if err = httpServer.Serve(httpListener); err != nil {
			panic(err)
		}
	}()
	go func() {
		if err := m.Serve(); err != nil {
			panic(err)
		}
	}()
	h.mux[lis] = m
	return
}

func (h *hostModule) ServerStop(ctx context.Context, lis net.Listener) (err error) {
	m, ok := h.mux[lis]
	if !ok {
		return
	}
	m.Close()
	return
}

func (h *hostModule) registerService(s *grpc.Server, pool wazeropool.Instance, meta *meta, serviceName string, methods []string, ctx context.Context, ctxCopiers ...ContextCopy) {
	handler := &grpcHandler{pool, meta, ctx, ctxCopiers}
	fakeDesc := &grpc.ServiceDesc{
		ServiceName: serviceName,
		HandlerType: (*any)(nil),
	}
	for _, m := range methods {
		parts := strings.Split(m, ".")
		if len(parts) < 2 {
			log.Panicf(`%s %#v`, methods, parts)
		}
		var d = grpc.StreamDesc{
			StreamName:    parts[1],
			ServerStreams: true,
			ClientStreams: true,
		}
		switch parts[0] {
		case "u":
			d.Handler = handler.handle(handlerFactoryUnary)
		case "c":
			d.Handler = handler.handle(handlerFactoryClientStream)
		case "s":
			d.Handler = handler.handle(handlerFactoryServerStream)
		case "b":
			d.Handler = handler.handle(handlerFactoryBidirectionalStream)
		}
		fakeDesc.Streams = append(fakeDesc.Streams, d)
	}
	s.RegisterService(fakeDesc, handler)
}

func (h *hostModule) Stop() (err error) {
	return
}

func get[T any](ctx context.Context, key string) T {
	v := ctx.Value(key)
	if v == nil {
		log.Panicf("Context item missing %s", key)
	}
	return v.(T)
}

func getErrCode(m api.Module, meta *meta) codes.Code {
	return codes.Code(readUint32(m, meta.ptrErrCode))
}

func setErrCode(m api.Module, meta *meta, code codes.Code) {
	writeUint32(m, meta.ptrErrCode, uint32(code))
}

func methodBuf(m api.Module, meta *meta) []byte {
	return read(m, meta.ptrMethod, 0, meta.ptrMethodCap)
}

func setMethod(m api.Module, meta *meta, method []byte) {
	copy(methodBuf(m, meta)[:len(method)], method)
	writeUint32(m, meta.ptrMethodLen, uint32(len(method)))
}

func getMsg(m api.Module, meta *meta) []byte {
	return read(m, meta.ptrMsg, meta.ptrMsgLen, meta.ptrMsgCap)
}

func getMsgCopy(m api.Module, meta *meta) []byte {
	return append([]byte(nil), getMsg(m, meta)...)
}

func msgBuf(m api.Module, meta *meta) []byte {
	return read(m, meta.ptrMsg, 0, meta.ptrMsgCap)
}

func setMsg(m api.Module, meta *meta, msg []byte) error {
	buf := msgBuf(m, meta)
	if len(msg) > cap(buf) {
		writeUint32(m, meta.ptrMsgLen, 0)
		return ErrMessageTooLarge
	}
	copy(buf[:len(msg)], msg)
	writeUint32(m, meta.ptrMsgLen, uint32(len(msg)))
	return nil
}

func getError(m api.Module, meta *meta, buf []byte) error {
	c := getErrCode(m, meta)
	if c != codes.OK {
		return status.New(c, string(buf)).Err()
	}
	return nil
}

func read(m api.Module, ptr, ptrLen, ptrCap uint32) (buf []byte) {
	buf, ok := m.Memory().Read(ptr, readUint32(m, ptrCap))
	if !ok {
		log.Panicf("Memory.Read(%d, %d) out of range", ptr, ptrCap)
	}
	return buf[:readUint32(m, ptrLen)]
}

func readUint32(m api.Module, ptr uint32) (val uint32) {
	val, ok := m.Memory().ReadUint32Le(ptr)
	if !ok {
		log.Panicf("Memory.Read(%d) out of range", ptr)
	}
	return
}

func writeUint32(m api.Module, ptr uint32, val uint32) {
	if ok := m.Memory().WriteUint32Le(ptr, val); !ok {
		log.Panicf("Memory.Read(%d) out of range", ptr)
	}
}
