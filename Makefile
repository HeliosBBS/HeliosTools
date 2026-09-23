# Recipes run under sh; $(shell ...) does not on Windows make, so nothing uses it.
SHELL := sh

.PHONY: check build vet lint test vuln bootstrap install

check: build vet lint test vuln

build:
	go build -trimpath ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# The race detector needs cgo and therefore a C compiler; CI always has one.
test:
	@command -v gcc >/dev/null 2>&1 || echo "WARNING: no C compiler found, race detector skipped"
	go test $$(command -v gcc >/dev/null 2>&1 && echo -race) -count=1 ./...

vuln:
	govulncheck ./...

bootstrap:
	go version
	golangci-lint version
	govulncheck -version

install:
	go install ./cmd/...
