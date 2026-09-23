# Recipes must run under both sh and cmd.exe: GNU Make on Windows ignores SHELL.
# The race detector needs a C compiler, so CI passes RACE=-race; pass it locally too
# when one is installed.
RACE ?=

.PHONY: check build vet lint test vuln bootstrap install

check: build vet lint test vuln

build:
	go build -trimpath ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

test:
	go test $(RACE) -count=1 ./...

vuln:
	govulncheck ./...

bootstrap:
	go version
	golangci-lint version
	govulncheck -version

install:
	go install ./cmd/...
