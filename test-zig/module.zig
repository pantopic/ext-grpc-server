const std = @import("std");
const atomic = @import("atomic");
const grpc_server = @import("grpc_server");
const pb = @import("pb/test.pb.zig");

const server = grpc_server.Server(.{
    .method_cap = 128,
    .msg_cap = 1536 * 1024,
    .services = &.{
        .{
            .name = "test.TestService",
            .methods = &.{
                .{ .name = "Test", .handler = .{ .unary = &test_ } },
                .{ .name = "Retest", .handler = .{ .unary = &retest } },
                .{ .name = "TestBytes", .handler = .{ .unary = &testBytes } },
                .{ .name = "ClientStream", .handler = .{ .client_stream = .{ .open = &csOpen, .recv = &csRecv, .close = &csClose } } },
                .{ .name = "ServerStream", .handler = .{ .server_stream = .{ .open = &ssOpen, .close = &ssClose } } },
                .{ .name = "BidirectionalStream", .handler = .{ .bidirectional = .{ .open = &bsOpen, .recv = &bsRecv, .close = &bsClose } } },
            },
        },
    },
    .http = &httpHandler,
});

comptime {
    server.register();
}

const counters = atomic.Uint64Set.init(0);
const counter1 = counters.find(1);
const counter2 = counters.find(2);

var scratch: [64 * 1024]u8 = undefined;
var fba = std.heap.FixedBufferAllocator.init(&scratch);
var out: [64 * 1024]u8 = undefined;

fn decode(comptime T: type, b: []const u8) anyerror!T {
    fba.reset();
    var reader = std.Io.Reader.fixed(b);
    return T.decode(&reader, fba.allocator()) catch error.InvalidArgument;
}

fn encode(msg: anytype) anyerror![]const u8 {
    var writer = std.Io.Writer.fixed(&out);
    try msg.encode(&writer, fba.allocator());
    return writer.buffered();
}

fn test_(b: []const u8) anyerror!void {
    const req = try decode(pb.TestRequest, b);
    const res = pb.TestResponse{ .bar = req.foo };
    try server.send(try encode(res));
}

fn retest(b: []const u8) anyerror!void {
    const req = try decode(pb.RetestRequest, b);
    const res = pb.RetestResponse{ .foo = req.bar };
    try server.send(try encode(res));
}

fn testBytes(b: []const u8) anyerror!void {
    _ = try decode(pb.TestBytesRequest, b);
    const res = pb.TestBytesResponse{ .code = 1, .data = "ACK" };
    try server.send(try encode(res));
}

fn csOpen() anyerror!void {
    counter1.store(0);
}

fn csRecv(b: []const u8) anyerror!void {
    const req = try decode(pb.ClientStreamRequest, b);
    _ = counter1.add(req.foo2);
}

fn csClose() anyerror!void {
    const res = pb.ClientStreamResponse{ .bar2 = counter1.load() };
    try server.send(try encode(res));
}

fn ssOpen(b: []const u8) anyerror!void {
    const req = try decode(pb.ServerStreamRequest, b);
    var n: u64 = 0;
    while (n < req.foo3) : (n += 1) {
        const res = pb.ServerStreamResponse{ .bar3 = n };
        try server.send(try encode(res));
    }
}

fn ssClose() anyerror!void {}

fn bsOpen() anyerror!void {
    counter2.store(0);
}

fn bsRecv(b: []const u8) anyerror!void {
    const req = try decode(pb.BidirectionalStreamRequest, b);
    const res = pb.BidirectionalStreamResponse{ .bar4 = counter2.add(req.foo4) };
    try server.send(try encode(res));
}

fn bsClose() anyerror!void {}

fn httpHandler(method: []const u8, path: []const u8, body: []const u8) grpc_server.HttpResponse {
    _ = method;
    _ = body;
    if (std.mem.eql(u8, path, "/hello")) {
        return .{ .code = 200, .body = "world" };
    }
    if (std.mem.eql(u8, path, "/goodbye")) {
        return .{ .code = 410, .body = "world" };
    }
    return .{};
}
