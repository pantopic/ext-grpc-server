# test-zig

The `test-zig` module uses [sdk-zig](../sdk-zig) and [zig-protobuf](https://github.com/Arwalk/zig-protobuf) for protobuf serialization.

Build the wasm module with `make wasm-zig` and regenerate [pb/test.pb.zig](pb/test.pb.zig) from [test.proto](../test.proto) with `make gen-test-zig`.

Requires zig `v0.16.0` or later.
