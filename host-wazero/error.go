package wazero_grpc_server

import (
	"google.golang.org/grpc/codes"
)

var ErrMessageTooLarge = Error{
	code: codes.InvalidArgument,
	msg:  "message too large",
}

type Error struct {
	code codes.Code
	msg  string
}

func (e Error) Error() string {
	return e.msg
}
