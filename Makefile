BINARY := gh-tool
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-s -w -X github.com/ascarter/gh-tool/cmd.version=$(VERSION)"

# Resolve the latest stable semver tag (vMAJOR.MINOR.PATCH).
# Computed inside each release recipe AFTER fetching remote tags so that
# bumping never trails origin.
define latest_tag
$(shell git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1)
endef

.PHONY: build test vet clean release release-patch release-minor release-major

build:
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
	go clean -cache -testcache

define check_release_ready
	@branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" != "main" ]; then \
		echo "error: must be on main branch (currently on $$branch)"; exit 1; \
	fi
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "error: working tree is not clean"; exit 1; \
	fi
	@git fetch origin --tags --prune --prune-tags --quiet; \
	local_sha=$$(git rev-parse HEAD); \
	remote_sha=$$(git rev-parse origin/main); \
	if [ "$$local_sha" != "$$remote_sha" ]; then \
		echo "error: local main is not in sync with origin/main"; exit 1; \
	fi
endef

release: release-patch

release-patch: test vet
	$(check_release_ready)
	$(eval LATEST_TAG := $(or $(call latest_tag),v0.0.0))
	$(eval NEXT_TAG := $(shell echo $(LATEST_TAG) | awk -F. '{print $$1"."$$2"."$$3+1}'))
	@echo "releasing $(NEXT_TAG) (was $(LATEST_TAG))"
	gh release create $(NEXT_TAG) --generate-notes

release-minor: test vet
	$(check_release_ready)
	$(eval LATEST_TAG := $(or $(call latest_tag),v0.0.0))
	$(eval NEXT_TAG := $(shell echo $(LATEST_TAG) | awk -F. '{print $$1"."$$2+1".0"}'))
	@echo "releasing $(NEXT_TAG) (was $(LATEST_TAG))"
	gh release create $(NEXT_TAG) --generate-notes

release-major: test vet
	$(check_release_ready)
	$(eval LATEST_TAG := $(or $(call latest_tag),v0.0.0))
	$(eval NEXT_TAG := $(shell echo $(LATEST_TAG) | awk -F. '{gsub(/^v/,"",$$1); print "v"$$1+1".0.0"}'))
	@echo "releasing $(NEXT_TAG) (was $(LATEST_TAG))"
	gh release create $(NEXT_TAG) --generate-notes
