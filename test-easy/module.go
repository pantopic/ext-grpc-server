package main

import (
	"github.com/pantopic/wazero-atomic/sdk-go"

	"github.com/pantopic/ext-grpc-server/sdk-go"
	"github.com/pantopic/ext-grpc-server/sdk-go/codes"
	"github.com/pantopic/ext-grpc-server/sdk-go/status"
	"github.com/pantopic/ext-grpc-server/test-easy/pb"
)

var (
	counters *atomic.Uint64Set
	counter1 *atomic.Uint64
	counter2 *atomic.Uint64

	uTestReq       = new(pb.TestRequest)
	uTestResp      = new(pb.TestResponse)
	uRetestReq     = new(pb.RetestRequest)
	uRetestResp    = new(pb.RetestResponse)
	uTestBytesReq  = new(pb.TestBytesRequest)
	uTestBytesResp = new(pb.TestBytesResponse)
	csReq          = new(pb.ClientStreamRequest)
	csResp         = new(pb.ClientStreamResponse)
	ssReq          = new(pb.ServerStreamRequest)
	ssResp         = new(pb.ServerStreamResponse)
	bsReq          = new(pb.BidirectionalStreamRequest)
	bsResp         = new(pb.BidirectionalStreamResponse)
)

func init() {
	counters = atomic.NewUint64Set(0)
	counter1 = counters.Find(1)
	counter2 = counters.Find(2)
	grpc_server.Init(
		grpc_server.WithHttpHandler(httpHandler),
	)
	grpc_server.NewService(`test.TestService`).
		Unary(`Test`, test).
		Unary(`Retest`, retest).
		Unary(`TestBytes`, testBytes).
		ClientStream(`ClientStream`, csOpen, csRecv, csClose).
		ServerStream(`ServerStream`, ssOpen, ssClose).
		BidirectionalStream(`BidirectionalStream`, bsOpen, bsRecv, bsClose)
}

func main() {}

func test(b []byte) (err error) {
	uTestReq.Reset()
	if err = uTestReq.Unmarshal(b); err != nil {
		err = status.New(codes.InvalidArgument, err.Error()).Err()
		return
	}
	uTestResp.Reset()
	uTestResp.Bar = uTestReq.Foo
	res := uTestResp.Marshal(nil)
	return grpc_server.Send(res)
}

func retest(b []byte) (err error) {
	uRetestReq.Reset()
	if err = uRetestReq.Unmarshal(b); err != nil {
		err = status.New(codes.InvalidArgument, err.Error()).Err()
		return
	}
	uRetestResp.Reset()
	uRetestResp.Foo = uRetestReq.Bar
	res := uRetestResp.Marshal(nil)
	return grpc_server.Send(res)
}

func testBytes(b []byte) (err error) {
	uTestBytesReq.Reset()
	if err = uTestBytesReq.Unmarshal(b); err != nil {
		err = status.New(codes.InvalidArgument, err.Error()).Err()
		return
	}
	uTestBytesResp.Reset()
	uTestBytesResp.Code = 1
	uTestBytesResp.Data = []byte(`ACK`)
	res := uTestBytesResp.Marshal(nil)
	return grpc_server.Send(res)
}

func csOpen() (err error) {
	counter1.Store(0)
	return
}

func csRecv(b []byte) (err error) {
	csReq.Reset()
	if err = csReq.Unmarshal(b); err != nil {
		err = status.New(codes.InvalidArgument, err.Error()).Err()
		return
	}
	counter1.Add(csReq.Foo2)
	return
}

func csClose() (err error) {
	csResp.Reset()
	csResp.Bar2 = counter1.Load()
	grpc_server.Send(csResp.Marshal(nil))
	return nil
}

func ssOpen(b []byte) (err error) {
	ssReq.Reset()
	if err = ssReq.Unmarshal(b); err != nil {
		err = status.New(codes.InvalidArgument, err.Error()).Err()
		return
	}
	var res []byte
	var n uint64
	for range ssReq.Foo3 {
		ssResp.Reset()
		ssResp.Bar3 = n
		res = ssResp.Marshal(res)
		if err = grpc_server.Send(res); err != nil {
			return
		}
		n++
	}
	return
}

func ssClose() (err error) {
	return
}

func bsOpen() (err error) {
	counter2.Store(0)
	return
}

func bsRecv(b []byte) (err error) {
	bsReq.Reset()
	if err = bsReq.Unmarshal(b); err != nil {
		err = status.New(codes.InvalidArgument, err.Error()).Err()
		return
	}
	bsResp.Reset()
	bsResp.Bar4 = counter2.Add(bsReq.Foo4)
	res := bsResp.Marshal(nil)
	return grpc_server.Send(res)
}

func bsClose() (err error) {
	return
}

func httpHandler(method, path, body []byte) (code int, res []byte) {
	switch string(path) {
	case "/hello":
		code = 200
		res = append(res, "world"...)
	case "/goodbye":
		code = 410
		res = append(res, "world"...)
	}
	return
}
