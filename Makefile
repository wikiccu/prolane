.PHONY: help build run fmt fmt-check test vet lint vuln secrets verify

help:
	@echo make build     - Build the CLI
	@echo make run       - Run the CLI
	@echo make fmt       - Format Go source files with gofmt
	@echo make fmt-check - Check formatting without changing files
	@echo make test      - Run Go tests
	@echo make vet       - Run Go static analysis
	@echo make lint      - Run the configured linters
	@echo make vuln      - Scan Go code for known vulnerabilities
	@echo make secrets   - Scan Git history for secrets with redacted output
	@echo make verify    - Check formatting, tests, vet, lint, and build

build:
	go build ./cmd/prolane

run:
	go run ./cmd/prolane

fmt:
	golangci-lint fmt

fmt-check:
	golangci-lint fmt --diff

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

vuln:
	govulncheck ./...

secrets:
	gitleaks git --redact --verbose --no-banner --log-opts="--all --full-history -m"

verify: fmt-check test vet lint build
