.PHONY: all fmt vet verify lint test build clean pre-pr install-hooks

all: verify test build

fmt:
	@git ls-files -z -- 'cmd/*.go' 'cmd/**/*.go' 'pkg/*.go' 'pkg/**/*.go' 'tests/*.go' 'tests/**/*.go' | xargs -0 gofmt -s -w

vet:
	@bash hack/verify-govet.sh

verify:
	@bash hack/verify-all.sh

lint:
	@VERIFY_GOLANGCI_LINT=1 bash hack/verify-all.sh

test:
	@go test -count=1 ./cmd/... ./pkg/... ./tests/...

build:
	@go build ./cmd/apiserver ./cmd/controller-manager ./cmd/node-agent ./cmd/registry-worker

clean:
	@rm -rf bin/

# Local pre-PR gate (remote CI is manual-only). Set PRE_PR_ALL=1 to run every suite.
pre-pr:
	@bash hack/pre-pr.sh

# Opt-in: run `make pre-pr` automatically on `git push` of feature branches.
install-hooks:
	@git config core.hooksPath .githooks
	@echo "Installed .githooks (pre-push runs hack/pre-pr.sh; bypass with --no-verify)."
