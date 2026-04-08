BINARY := gh-tool
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-s -w -X github.com/ascarter/gh-tool/cmd.version=$(VERSION)"

.PHONY: build test vet clean release

build:
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
	go clean -cache -testcache

release:
	@if [ -z "$(TAG)" ]; then echo "usage: make release TAG=v0.1.0"; exit 1; fi
	gh release create $(TAG) --generate-notes
