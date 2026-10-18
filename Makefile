.PHONY: build run check test-unit sqlc

build:
	mkdir -p bin
	go build -o bin/web-api ./cmd/web-api

run:
	node scripts/run.mjs

check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go build ./...
	go mod verify
	sqlc generate && git diff --exit-code -- internal/dbgen
	node --check scripts/run.mjs
	node --check scripts/crypto-fixture.cjs

# Needs Node and `npm ci --prefix tests/interop` for the Adonis token fixture.
test-unit:
	go test -race -count=1 ./...

sqlc:
	sqlc generate
