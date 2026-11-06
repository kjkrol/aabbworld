COMMIT_HASH := $(shell git rev-parse --short HEAD)
COMMIT_DATE := $(shell git log -1 --format=%cd --date=format:%Y%m%d)
DIRTY       := $(shell git diff --quiet || echo "-dirty")
RESULT_FILE := bench_results/bench_$(COMMIT_DATE)_$(COMMIT_HASH)$(DIRTY).txt

BENCH_COUNT ?= 5

.PHONY: test test-simd test-simd-128 test-simd-emulated vet-simd bench bench-simd bench-save bench-save-simd

## test: runs every test, examples included
test:
	go test ./...

## test-simd: runs every test built with the simd experiment, each wide kernel the CPU runs included
test-simd:
	GOEXPERIMENT=simd go test ./...

## test-simd-128: runs every test built with the simd experiment and simd's vectors held to 128 bits
test-simd-128:
	GOEXPERIMENT=simd GODEBUG=simd=128 go test ./...

## test-simd-emulated: runs every test built with the simd experiment and simd emulated: no kernel runs
test-simd-emulated:
	GOEXPERIMENT=simd GODEBUG=simd=0 go test ./...

## vet-simd: vets the simd build for amd64, arm64 and wasm, whatever this machine is
vet-simd:
	GOEXPERIMENT=simd GOARCH=amd64 go vet ./...
	GOEXPERIMENT=simd GOARCH=arm64 go vet ./...
	GOEXPERIMENT=simd GOOS=js GOARCH=wasm go vet ./...

## bench: runs the whole suite once, with allocations
bench:
	go test -bench=. -benchmem -count=1 ./bench/...

## bench-simd: runs the whole suite once, built with the simd experiment
bench-simd:
	GOEXPERIMENT=simd go test -bench=. -benchmem -count=1 ./bench/...

## bench-save: runs the suite BENCH_COUNT times and keeps the raw output under bench_results/
bench-save:
	@mkdir -p bench_results
	go test -bench=. -benchmem -count=$(BENCH_COUNT) ./bench/... > $(RESULT_FILE)
	@echo "Results saved into: $(RESULT_FILE)"

## bench-save-simd: bench-save built with the simd experiment, into a file marked -simd
bench-save-simd:
	@mkdir -p bench_results
	GOEXPERIMENT=simd go test -bench=. -benchmem -count=$(BENCH_COUNT) ./bench/... > $(RESULT_FILE:.txt=-simd.txt)
	@echo "Results saved into: $(RESULT_FILE:.txt=-simd.txt)"
