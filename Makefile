BINARY := gh-tool
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-s -w -X github.com/ascarter/gh-tool/cmd.version=$(VERSION)"

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

# do_release BUMP_AWK — shared release recipe parameterized by an awk
# expression that prints the next tag given the current one on stdin.
# Everything runs in one shell invocation so tag resolution sees the
# freshly fetched remote tags.
define do_release
	@set -e; \
	branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" != "main" ]; then \
		echo "error: must be on main branch (currently on $$branch)"; exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "error: working tree is not clean"; exit 1; \
	fi; \
	git fetch origin --tags --prune --prune-tags --quiet; \
	local_sha=$$(git rev-parse HEAD); \
	remote_sha=$$(git rev-parse origin/main); \
	if [ "$$local_sha" != "$$remote_sha" ]; then \
		echo "error: local main is not in sync with origin/main"; exit 1; \
	fi; \
	latest=$$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1); \
	latest=$${latest:-v0.0.0}; \
	next=$$(echo $$latest | awk -F. '$(1)'); \
	echo "releasing $$next (was $$latest)"; \
	gh release create $$next --generate-notes
endef

release: release-patch

release-patch: test vet
	$(call do_release,{print $$1"."$$2"."$$3+1})

release-minor: test vet
	$(call do_release,{print $$1"."$$2+1".0"})

release-major: test vet
	$(call do_release,{gsub(/^v/,"",$$1); print "v"$$1+1".0.0"})
