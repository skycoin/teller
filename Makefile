.DEFAULT_GOAL := help
.PHONY: teller test lint check format cover help

PACKAGES = $(shell find ./src -type d -not -path '\./src')

teller: ## Run teller. To add arguments, do 'make ARGS="--foo" teller'.
	go run cmd/teller/teller.go ${ARGS}

test: ## Run tests
	go test ./cmd/... -timeout=1m -cover
	go test ./src/... -timeout=1m -cover

lint: ## Run linters. Use make install-linters first.
	vendorcheck ./...
	gometalinter --deadline=3m -j 2 --disable-all --tests --vendor \
		-E deadcode \
		-E errcheck \
		-E gas \
		-E goconst \
		-E gofmt \
		-E goimports \
		-E golint \
		-E ineffassign \
		-E interfacer \
		-E maligned \
		-E megacheck \
		-E misspell \
		-E nakedret \
		-E structcheck \
		-E unconvert \
		-E unparam \
		-E varcheck \
		-E vet \
		./...

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
	# This sorts imports by [stdlib, 3rdpart, mdllife/teller]
	goimports -w -local github.com/mdllife/teller ./cmd
	goimports -w -local github.com/mdllife/teller ./src
	# This performs code simplifications
	gofmt -s -w ./cmd
	gofmt -s -w ./src

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

test-package: ## Run tests on each package
	go test -v ./cmd/... -timeout=1m -cover
	go test -v ./src/addrs/... -timeout=1m -cover
	go test -v ./src/config/... -timeout=1m -cover
	go test -v ./src/exchange/... -timeout=1m -cover
	go test -v ./src/monitor/... -timeout=1m -cover
	go test -v ./src/scanner/... -timeout=1m -cover
	go test -v ./src/sender/... -timeout=1m -cover
	go test -v ./src/teller/... -timeout=1m -cover
	go test -v ./src/util/... -timeout=1m -cover

test-btc-scanner:
	go test github.com/MDLlife/teller/src/scanner -v -run TestBtcScanner

test-eth-scanner:
	go test github.com/MDLlife/teller/src/scanner -v -run TestEthScanner

test-base-scanner:
	go test github.com/MDLlife/teller/src/scanner -v -run TestEthScanner

test-exchange:
	go test github.com/MDLlife/teller/src/exchange -v -run TestExchange

test-exchange-calculate:
	go test github.com/MDLlife/teller/src/exchange -v -run TestCalculate

test-exchange-store:
	go test github.com/MDLlife/teller/src/exchange -v -run TestStore