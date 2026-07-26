const std = @import("std");

pub fn build(b: *std.Build) void {
    _ = b.addModule("grpc_server", .{
        .root_source_file = b.path("grpc_server.zig"),
    });
}
