package grpc_server

import (
	"unsafe"

	"github.com/pantopic/ext-grpc-server/sdk-go/codes"
)

type Service struct {
	unary              map[string]unary
	clientStreamOpen   map[string]clientStreamOpen
	clientStreamRecv   map[string]clientStreamRecv
	clientStreamClose  map[string]clientStreamClose
	serverStreamOpen   map[string]serverStreamOpen
	serverStreamClose  map[string]serverStreamClose
	bidirectionalOpen  map[string]bidirectionalOpen
	bidirectionalRecv  map[string]bidirectionalRecv
	bidirectionalClose map[string]bidirectionalClose
}

type unary func([]byte) error
type clientStreamOpen func() error
type clientStreamRecv func([]byte) error
type clientStreamClose func() error
type serverStreamOpen func([]byte) error
type serverStreamClose func() error
type bidirectionalOpen func() error
type bidirectionalRecv func([]byte) error
type bidirectionalClose func() error

func NewService(name string) *Service {
	services[name] = &Service{
		unary:              make(map[string]unary),
		clientStreamOpen:   make(map[string]clientStreamOpen),
		clientStreamRecv:   make(map[string]clientStreamRecv),
		clientStreamClose:  make(map[string]clientStreamClose),
		serverStreamOpen:   make(map[string]serverStreamOpen),
		serverStreamClose:  make(map[string]serverStreamClose),
		bidirectionalOpen:  make(map[string]bidirectionalOpen),
		bidirectionalRecv:  make(map[string]bidirectionalRecv),
		bidirectionalClose: make(map[string]bidirectionalClose),
	}
	return services[name]
}

func (s *Service) Unary(name string, fn unary) *Service {
	s.unary[name] = fn
	return s
}

func (s *Service) ClientStream(name string,
	open clientStreamOpen,
	recv clientStreamRecv,
	close clientStreamClose,
) *Service {
	s.clientStreamOpen[name] = open
	s.clientStreamRecv[name] = recv
	s.clientStreamClose[name] = close
	return s
}

func (s *Service) ServerStream(name string,
	open serverStreamOpen,
	close serverStreamClose,
) *Service {
	s.serverStreamOpen[name] = open
	s.serverStreamClose[name] = close
	return s
}

func (s *Service) BidirectionalStream(name string,
	open bidirectionalOpen,
	recv bidirectionalRecv,
	close bidirectionalClose,
) *Service {
	s.bidirectionalRecv[name] = recv
	s.bidirectionalOpen[name] = open
	s.bidirectionalClose[name] = close
	return s
}

func Send(b []byte) error {
	return SendErr(codes.OK, b)
}

func SendErr(c codes.Code, b []byte) error {
	errCode = c
	res := uint64(len(b))
	if len(b) > 0 {
		res += uint64(uintptr(unsafe.Pointer(&b[0]))) << 32
	}
	send(res)
	return getErr()
}

// func Close() (err error) {
// 	errCode = codes.OK
// 	close()
// 	err = getErr()
// 	return
// }
