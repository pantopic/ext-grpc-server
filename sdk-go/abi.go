package grpc_server

import (
	"bytes"
	"sort"
	"strings"
	"unsafe"

	"github.com/pantopic/wazero-grpc-server/sdk-go/codes"
)

var (
	meta      = make([]uint32, 7)
	errCode   codes.Code
	method    []byte
	methodCap uint32 = 256
	methodLen uint32
	msg       []byte
	msgCap    uint32 = 1 * 1024 * 1024
	msgLen    uint32

	services    = map[string]*Service{}
	httpHandler httpHandlerFunc
)

type httpHandlerFunc func(method, path, body []byte) (code int, res []byte)

func Init(opts ...Option) {
	for _, opt := range opts {
		opt()
	}
	method = make([]byte, int(methodCap))
	msg = make([]byte, int(msgCap))
}

//export __grpc_server
func __grpc_server() (res uint32) {
	meta[0] = uint32(uintptr(unsafe.Pointer(&methodCap)))
	meta[1] = uint32(uintptr(unsafe.Pointer(&methodLen)))
	meta[2] = uint32(uintptr(unsafe.Pointer(&method[0])))
	meta[3] = uint32(uintptr(unsafe.Pointer(&msgCap)))
	meta[4] = uint32(uintptr(unsafe.Pointer(&msgLen)))
	meta[5] = uint32(uintptr(unsafe.Pointer(&msg[0])))
	meta[6] = uint32(uintptr(unsafe.Pointer(&errCode)))
	var serviceNames []string
	for k := range services {
		serviceNames = append(serviceNames, k)
	}
	sort.Strings(serviceNames)
	msg = msg[:0]
	for _, name := range serviceNames {
		msg = append(msg, []byte("/"+name+"/")...)
		var methods []string
		for k := range services[name].unary {
			methods = append(methods, `u.`+k)
		}
		for k := range services[name].clientStreamOpen {
			methods = append(methods, `c.`+k)
		}
		for k := range services[name].serverStreamOpen {
			methods = append(methods, `s.`+k)
		}
		for k := range services[name].bidirectionalRecv {
			methods = append(methods, `b.`+k)
		}
		sort.Strings(methods)
		msg = append(msg, []byte(strings.Join(methods, ","))...)
	}
	msgLen = uint32(len(msg))
	return uint32(uintptr(unsafe.Pointer(&meta[0])))
}

func setMsg(b []byte) {
	copy(msg[:len(b)], b)
	msgLen = uint32(len(b))
}

func getMsg() []byte {
	return msg[:msgLen]
}

func getMethod() []byte {
	return method[:methodLen]
}

func getCallOpts() (service *Service, method string) {
	m := string(getMethod())
	parts := strings.Split(m, "/")
	if len(parts) != 3 {
		errCode = codes.InvalidArgument
		setMsg([]byte(`Invalid method: ` + m))
		return
	}
	service, ok := services[parts[1]]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	method = parts[2]
	return
}

//export __grpc_server_unary
func __grpc_server_unary() {
	service, method := getCallOpts()
	h, ok := service.unary[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h(getMsg())
	check(err)
}

//export __grpc_server_client_stream_open
func __grpc_server_client_stream_open() {
	service, method := getCallOpts()
	h, ok := service.clientStreamOpen[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h()
	check(err)
}

//export __grpc_server_client_stream_recv
func __grpc_server_client_stream_recv() {
	service, method := getCallOpts()
	h, ok := service.clientStreamRecv[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h(getMsg())
	check(err)
}

//export __grpc_server_client_stream_close
func __grpc_server_client_stream_close() {
	service, method := getCallOpts()
	h, ok := service.clientStreamClose[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h()
	check(err)
}

//export __grpc_server_server_stream_open
func __grpc_server_server_stream_open() {
	service, method := getCallOpts()
	h, ok := service.serverStreamOpen[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h(getMsg())
	check(err)
}

//export __grpc_server_server_stream_close
func __grpc_server_server_stream_close() {
	service, method := getCallOpts()
	h, ok := service.serverStreamClose[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h()
	check(err)
}

//export __grpc_server_bidirectional_open
func __grpc_server_bidirectional_open() {
	service, method := getCallOpts()
	h, ok := service.bidirectionalOpen[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h()
	check(err)
}

//export __grpc_server_bidirectional_recv
func __grpc_server_bidirectional_recv() {
	service, method := getCallOpts()
	h, ok := service.bidirectionalRecv[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h(getMsg())
	check(err)
}

//export __grpc_server_bidirectional_close
func __grpc_server_bidirectional_close() {
	service, method := getCallOpts()
	h, ok := service.bidirectionalClose[method]
	if !ok {
		errCode = codes.Unimplemented
		return
	}
	err := h()
	check(err)
}

func getHttpMethod() (method, path []byte) {
	parts := bytes.Split(getMethod(), []byte{' '})
	if len(parts) != 2 {
		errCode = codes.InvalidArgument
		setMsg([]byte(`Invalid method: ` + string(getMethod())))
		return
	}
	method = parts[0]
	path = parts[1]
	return
}

//export __grpc_server_http
func __grpc_server_http() {
	if httpHandler == nil {
		errCode = codes.Code(405)
		return
	}
	method, path := getHttpMethod()
	code, b := httpHandler(method, path, getMsg())
	setMsg(b)
	errCode = codes.Code(uint32(code))
}

//go:wasm-module pantopic/wazero-grpc-server
//export __grpc_server_send
func send(uint64)

//go:wasm-module pantopic/wazero-grpc-server
//export __grpc_server_close
// func close()

// Fix for lint rule `unusedfunc`
var _ = __grpc_server
var _ = __grpc_server_unary
var _ = __grpc_server_client_stream_open
var _ = __grpc_server_client_stream_recv
var _ = __grpc_server_client_stream_close
var _ = __grpc_server_server_stream_open
var _ = __grpc_server_server_stream_close
var _ = __grpc_server_bidirectional_open
var _ = __grpc_server_bidirectional_recv
var _ = __grpc_server_bidirectional_close
var _ = __grpc_server_http
