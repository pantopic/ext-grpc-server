const std = @import("std");

extern "pantopic/wazero-grpc-server" fn __grpc_server_send(u64) void;

pub const UnaryFn = *const fn (req: []const u8) anyerror!void;
pub const OpenFn = *const fn () anyerror!void;
pub const RecvFn = *const fn (req: []const u8) anyerror!void;
pub const CloseFn = *const fn () anyerror!void;

pub const Handler = union(enum) {
    unary: UnaryFn,
    client_stream: struct { open: OpenFn, recv: RecvFn, close: CloseFn },
    server_stream: struct { open: RecvFn, close: CloseFn },
    bidirectional: struct { open: OpenFn, recv: RecvFn, close: CloseFn },
};

pub const Method = struct {
    name: []const u8,
    handler: Handler,
};

pub const Service = struct {
    name: []const u8,
    methods: []const Method,
};

pub const HttpResponse = struct {
    code: u32 = 0,
    body: []const u8 = "",
};

pub const HttpHandler = *const fn (method: []const u8, path: []const u8, body: []const u8) HttpResponse;

pub const Config = struct {
    method_cap: u32 = 256,
    msg_cap: u32 = 1024 * 1024,
    services: []const Service,
    http: ?HttpHandler = null,
};

pub const Error = error{
    Cancelled,
    Unknown,
    InvalidArgument,
    DeadlineExceeded,
    NotFound,
    AlreadyExists,
    PermissionDenied,
    ResourceExhausted,
    FailedPrecondition,
    Aborted,
    OutOfRange,
    Unimplemented,
    Internal,
    Unavailable,
    DataLoss,
    Unauthenticated,
};

pub const code_ok: u32 = 0;
pub const code_invalid_argument: u32 = 3;
pub const code_unimplemented: u32 = 12;

fn codeOf(err: anyerror) u32 {
    return switch (err) {
        error.Cancelled => 1,
        error.Unknown => 2,
        error.InvalidArgument => 3,
        error.DeadlineExceeded => 4,
        error.NotFound => 5,
        error.AlreadyExists => 6,
        error.PermissionDenied => 7,
        error.ResourceExhausted, error.OutOfMemory => 8,
        error.FailedPrecondition => 9,
        error.Aborted => 10,
        error.OutOfRange => 11,
        error.Unimplemented => 12,
        error.Internal => 13,
        error.Unavailable => 14,
        error.DataLoss => 15,
        error.Unauthenticated => 16,
        else => 2,
    };
}

fn errOf(code: u32) anyerror {
    return switch (code) {
        1 => error.Cancelled,
        3 => error.InvalidArgument,
        4 => error.DeadlineExceeded,
        5 => error.NotFound,
        6 => error.AlreadyExists,
        7 => error.PermissionDenied,
        8 => error.ResourceExhausted,
        9 => error.FailedPrecondition,
        10 => error.Aborted,
        11 => error.OutOfRange,
        12 => error.Unimplemented,
        13 => error.Internal,
        14 => error.Unavailable,
        15 => error.DataLoss,
        16 => error.Unauthenticated,
        else => error.Unknown,
    };
}

const Kind = enum {
    unary,
    client_stream_open,
    client_stream_recv,
    client_stream_close,
    server_stream_open,
    server_stream_close,
    bidirectional_open,
    bidirectional_recv,
    bidirectional_close,
};

fn stringLessThan(a: []const u8, b: []const u8) bool {
    return std.mem.lessThan(u8, a, b);
}

fn sortStrings(comptime n: usize, items: *[n][]const u8) void {
    for (1..n) |i| {
        var j = i;
        while (j > 0 and stringLessThan(items[j], items[j - 1])) : (j -= 1) {
            std.mem.swap([]const u8, &items[j], &items[j - 1]);
        }
    }
}

fn methodPrefix(comptime h: Handler) []const u8 {
    return switch (h) {
        .unary => "u",
        .client_stream => "c",
        .server_stream => "s",
        .bidirectional => "b",
    };
}

// Builds the service directory string read by the host on `__grpc_server`.
// Format: "/package1.ServiceName/c.method1,u.method2/package2.ServiceName/b.method1,s.method2"
fn directory(comptime cfg: Config) []const u8 {
    comptime {
        var names: [cfg.services.len][]const u8 = undefined;
        for (cfg.services, 0..) |svc, i| names[i] = svc.name;
        sortStrings(names.len, &names);
        var out: []const u8 = "";
        for (names) |name| {
            for (cfg.services) |svc| {
                if (!std.mem.eql(u8, svc.name, name)) continue;
                var methods: [svc.methods.len][]const u8 = undefined;
                for (svc.methods, 0..) |m, i| {
                    methods[i] = methodPrefix(m.handler) ++ "." ++ m.name;
                }
                sortStrings(methods.len, &methods);
                out = out ++ "/" ++ name ++ "/";
                for (methods, 0..) |m, i| {
                    if (i > 0) out = out ++ ",";
                    out = out ++ m;
                }
            }
        }
        return out;
    }
}

fn handlerMatches(comptime h: Handler, comptime kind: Kind) bool {
    return switch (kind) {
        .unary => h == .unary,
        .client_stream_open, .client_stream_recv, .client_stream_close => h == .client_stream,
        .server_stream_open, .server_stream_close => h == .server_stream,
        .bidirectional_open, .bidirectional_recv, .bidirectional_close => h == .bidirectional,
    };
}

pub fn Server(comptime cfg: Config) type {
    return struct {
        var method_cap: u32 = cfg.method_cap;
        var method_len: u32 = 0;
        var method_buf: [cfg.method_cap]u8 = undefined;
        var msg_cap: u32 = cfg.msg_cap;
        var msg_len: u32 = 0;
        var msg_buf: [cfg.msg_cap]u8 = undefined;
        var err_code: u32 = 0;
        var meta: [7]u32 = undefined;

        /// Exports the guest ABI expected by the host module.
        /// Must be called from a comptime block in the root module.
        pub fn register() void {
            @export(&expMeta, .{ .name = "__grpc_server" });
            @export(&expUnary, .{ .name = "__grpc_server_unary" });
            @export(&expClientStreamOpen, .{ .name = "__grpc_server_client_stream_open" });
            @export(&expClientStreamRecv, .{ .name = "__grpc_server_client_stream_recv" });
            @export(&expClientStreamClose, .{ .name = "__grpc_server_client_stream_close" });
            @export(&expServerStreamOpen, .{ .name = "__grpc_server_server_stream_open" });
            @export(&expServerStreamClose, .{ .name = "__grpc_server_server_stream_close" });
            @export(&expBidirectionalOpen, .{ .name = "__grpc_server_bidirectional_open" });
            @export(&expBidirectionalRecv, .{ .name = "__grpc_server_bidirectional_recv" });
            @export(&expBidirectionalClose, .{ .name = "__grpc_server_bidirectional_close" });
            @export(&expHttp, .{ .name = "__grpc_server_http" });
        }

        pub fn send(b: []const u8) anyerror!void {
            return sendErr(code_ok, b);
        }

        pub fn sendErr(code: u32, b: []const u8) anyerror!void {
            err_code = code;
            var res: u64 = b.len;
            if (res > 0) {
                res = res + (@as(u64, @intFromPtr(&b[0])) << 32);
            }
            __grpc_server_send(res);
            return getErr();
        }

        fn expMeta() callconv(.c) u32 {
            meta[0] = @intFromPtr(&method_cap);
            meta[1] = @intFromPtr(&method_len);
            meta[2] = @intFromPtr(&method_buf);
            meta[3] = @intFromPtr(&msg_cap);
            meta[4] = @intFromPtr(&msg_len);
            meta[5] = @intFromPtr(&msg_buf);
            meta[6] = @intFromPtr(&err_code);
            const dir = comptime directory(cfg);
            setMsg(dir);
            return @intFromPtr(&meta);
        }

        fn expUnary() callconv(.c) void {
            dispatch(.unary);
        }

        fn expClientStreamOpen() callconv(.c) void {
            dispatch(.client_stream_open);
        }

        fn expClientStreamRecv() callconv(.c) void {
            dispatch(.client_stream_recv);
        }

        fn expClientStreamClose() callconv(.c) void {
            dispatch(.client_stream_close);
        }

        fn expServerStreamOpen() callconv(.c) void {
            dispatch(.server_stream_open);
        }

        fn expServerStreamClose() callconv(.c) void {
            dispatch(.server_stream_close);
        }

        fn expBidirectionalOpen() callconv(.c) void {
            dispatch(.bidirectional_open);
        }

        fn expBidirectionalRecv() callconv(.c) void {
            dispatch(.bidirectional_recv);
        }

        fn expBidirectionalClose() callconv(.c) void {
            dispatch(.bidirectional_close);
        }

        fn expHttp() callconv(.c) void {
            const h = cfg.http orelse {
                err_code = 405;
                return;
            };
            const m = getMethod();
            const idx = std.mem.indexOfScalar(u8, m, ' ') orelse {
                err_code = code_invalid_argument;
                setInvalidMethod(m);
                return;
            };
            const res = h(m[0..idx], m[idx + 1 ..], getMsg());
            setMsg(res.body);
            err_code = res.code;
        }

        fn dispatch(comptime kind: Kind) void {
            const c = getCall() orelse return;
            inline for (cfg.services) |svc| {
                if (std.mem.eql(u8, svc.name, c.service)) {
                    inline for (svc.methods) |m| {
                        if (comptime handlerMatches(m.handler, kind)) {
                            if (std.mem.eql(u8, m.name, c.method)) {
                                check(invoke(m.handler, kind));
                                return;
                            }
                        }
                    }
                }
            }
            err_code = code_unimplemented;
        }

        fn invoke(comptime h: Handler, comptime kind: Kind) anyerror!void {
            return switch (kind) {
                .unary => h.unary(getMsg()),
                .client_stream_open => h.client_stream.open(),
                .client_stream_recv => h.client_stream.recv(getMsg()),
                .client_stream_close => h.client_stream.close(),
                .server_stream_open => h.server_stream.open(getMsg()),
                .server_stream_close => h.server_stream.close(),
                .bidirectional_open => h.bidirectional.open(),
                .bidirectional_recv => h.bidirectional.recv(getMsg()),
                .bidirectional_close => h.bidirectional.close(),
            };
        }

        const Call = struct {
            service: []const u8,
            method: []const u8,
        };

        fn getCall() ?Call {
            const m = getMethod();
            if (m.len < 3 or m[0] != '/') return invalidMethod(m);
            const idx = std.mem.indexOfScalarPos(u8, m, 1, '/') orelse return invalidMethod(m);
            if (std.mem.indexOfScalarPos(u8, m, idx + 1, '/') != null) return invalidMethod(m);
            return .{ .service = m[1..idx], .method = m[idx + 1 ..] };
        }

        fn invalidMethod(m: []const u8) ?Call {
            err_code = code_invalid_argument;
            setInvalidMethod(m);
            return null;
        }

        fn setInvalidMethod(m: []const u8) void {
            const prefix = "Invalid method: ";
            std.mem.copyForwards(u8, msg_buf[0..prefix.len], prefix);
            std.mem.copyForwards(u8, msg_buf[prefix.len..][0..m.len], m);
            msg_len = @intCast(prefix.len + m.len);
        }

        fn check(res: anyerror!void) void {
            if (res) |_| {
                err_code = code_ok;
            } else |err| {
                err_code = codeOf(err);
                setMsg(@errorName(err));
            }
        }

        fn getErr() anyerror!void {
            if (err_code != code_ok) return errOf(err_code);
        }

        fn getMethod() []const u8 {
            return method_buf[0..method_len];
        }

        fn getMsg() []const u8 {
            return msg_buf[0..msg_len];
        }

        fn setMsg(b: []const u8) void {
            std.mem.copyForwards(u8, msg_buf[0..b.len], b);
            msg_len = @intCast(b.len);
        }
    };
}
