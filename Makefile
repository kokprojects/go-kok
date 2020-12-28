# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: gkok android ios gkok-cross swarm evm all test clean
.PHONY: gkok-linux gkok-linux-386 gkok-linux-amd64 gkok-linux-mips64 gkok-linux-mips64le
.PHONY: gkok-linux-arm gkok-linux-arm-5 gkok-linux-arm-6 gkok-linux-arm-7 gkok-linux-arm64
.PHONY: gkok-darwin gkok-darwin-386 gkok-darwin-amd64
.PHONY: gkok-windows gkok-windows-386 gkok-windows-amd64

GOBIN = $(shell pwd)/build/bin
GO ?= latest

gkok:
	build/env.sh go run build/ci.go install ./cmd/gkok
	@echo "Done building."
	@echo "Run \"$(GOBIN)/gkok\" to launch gkok."

swarm:
	build/env.sh go run build/ci.go install ./cmd/swarm
	@echo "Done building."
	@echo "Run \"$(GOBIN)/swarm\" to launch swarm."

all:
	build/env.sh go run build/ci.go install

android:
	build/env.sh go run build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/gkok.aar\" to use the library."

ios:
	build/env.sh go run build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/Gkok.framework\" to use the library."

test: all
	build/env.sh go run build/ci.go test

clean:
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	env GOBIN= go get -u golang.org/x/tools/cmd/stringer
	env GOBIN= go get -u github.com/jteeuwen/go-bindata/go-bindata
	env GOBIN= go get -u github.com/fjl/gencodec
	env GOBIN= go install ./cmd/abigen

# Cross Compilation Targets (xgo)

gkok-cross: gkok-linux gkok-darwin gkok-windows gkok-android gkok-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/gkok-*

gkok-linux: gkok-linux-386 gkok-linux-amd64 gkok-linux-arm gkok-linux-mips64 gkok-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-*

gkok-linux-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/gkok
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep 386

gkok-linux-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/gkok
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep amd64

gkok-linux-arm: gkok-linux-arm-5 gkok-linux-arm-6 gkok-linux-arm-7 gkok-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep arm

gkok-linux-arm-5:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/gkok
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep arm-5

gkok-linux-arm-6:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/gkok
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep arm-6

gkok-linux-arm-7:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v ./cmd/gkok
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep arm-7

gkok-linux-arm64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/gkok
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep arm64

gkok-linux-mips:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/gkok
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep mips

gkok-linux-mipsle:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/gkok
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep mipsle

gkok-linux-mips64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/gkok
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep mips64

gkok-linux-mips64le:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/gkok
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/gkok-linux-* | grep mips64le

gkok-darwin: gkok-darwin-386 gkok-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/gkok-darwin-*

gkok-darwin-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/386 -v ./cmd/gkok
	@echo "Darwin 386 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-darwin-* | grep 386

gkok-darwin-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/gkok
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-darwin-* | grep amd64

gkok-windows: gkok-windows-386 gkok-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/gkok-windows-*

gkok-windows-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/gkok
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-windows-* | grep 386

gkok-windows-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/gkok
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/gkok-windows-* | grep amd64
