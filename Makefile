.DEFAULT_GOAL := help
.PHONY: proxy teller install-skycoin-cli test lint check format cover help

PACKAGES = $(shell find ./src -type d -not -path '\./src')

proxy:  ## Run the teller proxy. To add arguments, do 'make ARGS="--foo" proxy'.
	go run cmd/proxy/proxy.go ${ARGS}

teller: SKYCOIN-CLI-exists ## Run teller. To add arguments, do 'make ARGS="--foo" teller'.
	go run cmd/teller/teller.go ${ARGS}

install-skycoin-cli: ## Install skycoin-cli
	go get github.com/skycoin/skycoin/cmd/cli
	@mv $$GOPATH/bin/cli $$GOPATH/bin/skycoin-cli

test: ## Run tests
	go test ./cmd/... -timeout=1m -cover
	go test ./src/... -timeout=1m -cover

lint: ## Run linters
	gometalinter --disable-all -E goimports --tests --vendor ./...
	vendorcheck ./...

check: lint test ## Run tests and linters

cover: ## Runs tests on ./src/ with HTML code coverage
	@echo "mode: count" > coverage-all.out
	$(foreach pkg,$(PACKAGES),\
		go test -coverprofile=coverage.out $(pkg);\
		tail -n +2 coverage.out >> coverage-all.out;)
	go tool cover -html=coverage-all.out

install-linters: ## Install linters
	go get -u -f github.com/golang/lint/golint
	go get -u -f golang.org/x/tools/cmd/goimports
	go get -u github.com/alecthomas/gometalinter
	go get -u github.com/FiloSottile/vendorcheck

format:  # Formats the code. Must have goimports installed (use make install-linters).
	goimports -w ./cmd/...
	goimports -w ./src/...

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
