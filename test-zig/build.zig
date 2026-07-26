const std = @import("std");
const protobuf = @import("protobuf");

pub fn build(b: *std.Build) void {
    const target = b.resolveTargetQuery(.{
        .cpu_arch = .wasm32,
        .os_tag = .freestanding,
    });
    const optimize = b.standardOptimizeOption(.{
        .preferred_optimize_mode = .ReleaseSmall,
    });
    const protobuf_dep = b.dependency("protobuf", .{});
    const grpc_dep = b.dependency("grpc_sdk_zig", .{});
    const atomic_dep = b.dependency("atomic_sdk_zig", .{});
    const exe = b.addExecutable(.{
        .name = "test-zig",
        .root_module = b.createModule(.{
            .root_source_file = b.path("module.zig"),
            .target = target,
            .optimize = optimize,
            .imports = &.{
                .{ .name = "grpc_server", .module = grpc_dep.module("grpc_server") },
                .{ .name = "atomic", .module = atomic_dep.module("atomic") },
                .{ .name = "protobuf", .module = protobuf_dep.module("protobuf") },
            },
        }),
    });
    exe.entry = .disabled;
    exe.rdynamic = true;
    b.installArtifact(exe);

    const gen_proto = b.step("gen-proto", "generates zig files from protocol buffer definitions");
    const protoc_step = protobuf.RunProtocStep.create(protobuf_dep.builder, b.graph.host, .{
        .destination_directory = b.path("pb"),
        .source_files = &.{
            b.path("../test.proto"),
        },
        .include_directories = &.{
            b.path(".."),
        },
    });
    gen_proto.dependOn(&protoc_step.step);
}
