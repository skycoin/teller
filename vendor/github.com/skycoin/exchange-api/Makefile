.DEFAULT_GOAL := help
.PHONY: exchange-api-server test lint lint-fast check format cover help

PACKAGES = $(shell ./packages.sh)

AVAILABLE_TAGS = $(shell ./available-tags.sh)

exchange-api-server:
	go run cmd/exchange-api-server/exchange-api-server.go ${ARGS}

test:
	go test ./rpc/... -timeout=1m -cover -tags "${AVAILABLE_TAGS}"
	go test ./cli/... -timeout=1m -cover -tags "${AVAILABLE_TAGS}"
	go test ./cmd/... -timeout=1m -cover -tags "${AVAILABLE_TAGS}"
	go test ./db/... -timeout=1m -cover -tags "${AVAILABLE_TAGS}"
	go test ./exchange/... -timeout=1m -cover -tags "${AVAILABLE_TAGS}"

lint: ## Run linters. Use make install-linters first.
	vendorcheck ./...
	gometalinter --deadline=3m -j 2 --disable-all --tests --vendor \
		-E goimports \
		-E unparam \
		-E deadcode \
		-E errcheck \
		-E gas \
		-E goconst \
		-E gofmt \
		-E golint \
		-E ineffassign \
		-E interfacer \
		-E maligned \
		-E megacheck \
		-E misspell \
		-E nakedret \
		-E structcheck \
		-E unconvert \
		-E varcheck \
		-E vet \
		./...

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

	goimports -w -local github.com/skycoin/exchange-api ./cli
	goimports -w -local github.com/skycoin/exchange-api ./cmd
	goimports -w -local github.com/skycoin/exchange-api ./db
	goimports -w -local github.com/skycoin/exchange-api ./exchange
	goimports -w -local github.com/skycoin/exchange-api ./rpc

	goimports -w -local github.com/skycoin/skycoin ./cli
	goimports -w -local github.com/skycoin/skycoin ./cmd
	goimports -w -local github.com/skycoin/skycoin ./db
	goimports -w -local github.com/skycoin/skycoin ./exchange
	goimports -w -local github.com/skycoin/skycoin ./rpc

	gofmt -s -w ./cli
	gofmt -s -w ./cmd
	gofmt -s -w ./db
	gofmt -s -w ./exchange
	gofmt -s -w ./rpc

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
