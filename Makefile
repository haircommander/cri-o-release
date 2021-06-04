export GOPROXY=https://proxy.golang.org
export GOSUMDB=https://sum.golang.org

GO ?= go
GO_BUILD ?= $(GO) build
GO_RUN ?= $(GO) run
PROJECT := github.com/haircommander/cri-o-release
GO_FILES := $(shell find . -type f -name '*.go' -not -name '*_test.go')

# If GOPATH not specified, use one in the local directory
ifeq ($(GOPATH),)
export GOPATH := $(CURDIR)/_output
unexport GOBIN
endif


all: binaries

binaries: bin/cri-o-release

bin/cri-o-release: $(GO_FILES)
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ ./cmd/cri-o-release/

vendor: export GOSUMDB :=
vendor: $(GO_FILES)
	$(GO) mod tidy
	$(GO) mod vendor
	$(GO) mod verify

clean:
	rm -rf _output
	rm -rf bin/


