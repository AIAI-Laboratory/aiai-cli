.PHONY: all build test fmt fmt-check lint setup

all: build

setup:
	@npm install

build:
	@./scripts/build.sh

test:
	@./scripts/test.sh

fmt:
	@./scripts/fmt.sh

fmt-check:
	@./scripts/fmt.sh --check

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run
