work:
	go work use sdk-go
	go work use test-easy
	go work use test-lite
	go work use host-wazero

wasm-easy:
	@cd test-easy && tinygo build -buildmode=wasi-legacy -target=wasi -opt=2 -gc=leaking -scheduler=none -o ../host-wazero/test-easy.wasm
wasm-easy-prod:
	@cd test-easy && tinygo build -buildmode=wasi-legacy -target=wasi -opt=s -gc=leaking -scheduler=none -o ../host-wazero/test-easy.prod.wasm -no-debug
wasm-lite:
	@cd test-lite && tinygo build -buildmode=wasi-legacy -target=wasi -opt=2 -gc=leaking -scheduler=none -o ../host-wazero/test-lite.wasm
wasm-lite-prod:
	@cd test-lite && tinygo build -buildmode=wasi-legacy -target=wasi -opt=s -gc=leaking -scheduler=none -o ../host-wazero/test-lite.prod.wasm -no-debug
wasm-zig:
	@cd test-zig && zig build --release=small
	@cp test-zig/zig-out/bin/test-zig.wasm host-wazero/test-zig.wasm
wasm-go: wasm-dev wasm-prod
wasm-dev: wasm-easy wasm-lite
wasm-prod: wasm-easy-prod wasm-lite-prod

wasm: wasm-go wasm-zig

test:
	@cd host-wazero && go test . -v -cover

bench:
	@cd host-wazero && go test -bench=. -v -run=Benchmark.*

cover:
	@mkdir -p _dist
	@cd host-wazero && go test . -coverprofile=../_dist/coverage.out -v
	@go tool cover -html=_dist/coverage.out -o _dist/coverage.html

gen-go:
	@protoc test.proto --go_out=host-wazero/pb \
		--go_opt=paths=source_relative \
		--go-grpc_opt=paths=source_relative \
		--go-grpc_out=host-wazero/pb

gen-install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

gen-test-lite:
	@ protoc test.proto \
		--plugin protoc-gen-go-lite="${GOBIN}/protoc-gen-go-lite" \
		--go-lite_out=test-lite/pb \
		--go-lite_opt=features=marshal+unmarshal+size \
		--go-lite_opt=paths=source_relative

gen-test-lite-install:
	go install github.com/aperturerobotics/protobuf-go-lite/cmd/protoc-gen-go-lite@latest

gen-test-zig:
	@cd test-zig && zig build gen-proto

gen: gen-go gen-test-lite gen-test-zig

cloc:
	@cloc . --exclude-dir=_example,_dist,internal,cmd --exclude-ext=pb.go

.PHONY: all test clean
