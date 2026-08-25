package grpc_server

import (
	"github.com/pantopic/ext-grpc-server/sdk-go/codes"
	"github.com/pantopic/ext-grpc-server/sdk-go/status"
)

type Error interface {
	Code() codes.Code
	Message() string
	Error() string
}

func check(err error) bool {
	if err == nil {
		errCode = codes.OK
		return false
	}
	if err, ok := err.(Error); ok {
		errCode = err.Code()
	} else {
		errCode = codes.Unknown
	}
	setMsg([]byte(err.Error()))
	return true
}

func getErr() (err error) {
	if errCode != codes.OK {
		err = status.New(errCode, string(getMsg())).Err()
	}
	return
}
