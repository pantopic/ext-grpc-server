# sdk-zig

Zig guest SDK for `pantopic/wazero-grpc-server`, equivalent to [sdk-go](../sdk-go).

Services are declared at comptime and exported to the host via `server.register()`:

```zig
const grpc_server = @import("grpc_server");

const server = grpc_server.Server(.{
    .method_cap = 128,
    .msg_cap = 1536 * 1024,
    .services = &.{
        .{
            .name = "test.TestService",
            .methods = &.{
                .{ .name = "Test", .handler = .{ .unary = &test_ } },
            },
        },
    },
    .http = &httpHandler,
});

comptime {
    server.register();
}
```

Handlers return `anyerror!void`; errors named after gRPC codes (eg. `error.InvalidArgument`)
are mapped to the corresponding status code. Responses are sent with `server.send(bytes)`.

Requires zig `v0.16.0` or later.
