.PHONY: build run check test-unit test-contract sqlc

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
	rm -rf .artifacts/dbgen && mkdir -p .artifacts && cp -R internal/dbgen .artifacts/dbgen && sqlc generate && diff -r .artifacts/dbgen internal/dbgen
	node --check scripts/run.mjs
	node --check scripts/crypto-fixture.cjs
	node --check scripts/vine-fixture.mjs
	node --check scripts/edge-fixture.mjs
	node --check scripts/contract-auth.mjs
	go vet -tags=integration ./...

# Needs Node and `npm ci --prefix tests/interop` for the Adonis token fixture.
test-unit:
	go test -race -count=1 ./...

# Differential check against Adonis web-be in an owned schema (docs/MIGRATION.md).
test-contract:
	node scripts/contract-auth.mjs

sqlc:
	sqlc generate
