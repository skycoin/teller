.DEFAULT_GOAL := help
.PHONY: teller test lint lint-fast check format cover help

PACKAGES = $(shell find ./src -type d -not -path '\./src')

teller: ## Run teller. To add arguments, do 'make ARGS="--foo" teller'.
	go run cmd/teller/teller.go ${ARGS}

test: ## Run tests
	go test ./cmd/... -timeout=1m -cover
	go test ./src/... -timeout=1m -cover

lint: ## Run linters. Use make install-linters first.
	vendorcheck ./...
	gometalinter --deadline=2m --disable-all -E gofmt -E goimports -E unparam --tests --vendor ./...

lint-fast: ## Run linters. Use make install-linters first. Skips slow linters.
	vendorcheck ./...
	gometalinter --disable-all -E goimports --tests --vendor ./...

check: lint test ## Run tests and linters

cover: ## Runs tests on ./src/ with HTML code coverage
	@echo "mode: count" > coverage-all.out
	$(foreach pkg,$(PACKAGES),\
		go test -coverprofile=coverage.out $(pkg);\
		tail -n +2 coverage.out >> coverage-all.out;)
	go tool cover -html=coverage-all.out

install-linters: ## Install linters
	go get -u github.com/FiloSottile/vendorcheck
	go get -u github.com/alecthomas/gometalinter
	gometalinter --vendored-linters --install

format:  # Formats the code. Must have goimports installed (use make install-linters).
	# This sorts imports by [stdlib, 3rdpart, skycoin/skycoin, skycoin/teller]
	goimports -w -local github.com/skycoin/teller ./cmd
	goimports -w -local github.com/skycoin/teller ./src
	goimports -w -local github.com/skycoin/skycoin ./cmd
	goimports -w -local github.com/skycoin/skycoin ./src
	# This performs code simplifications
	gofmt -s -w ./cmd
	gofmt -s -w ./cmd

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
