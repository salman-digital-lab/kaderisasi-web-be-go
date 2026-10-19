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
	node --check scripts/contract-lib.mjs
	node --check scripts/contract-public.mjs
	node --check scripts/sanitize-fixture.mjs
	node --check scripts/contract-members.mjs
	node --check scripts/contract-forms.mjs
	node --check scripts/contract-courses.mjs
	node --check scripts/contract-routing.mjs
	node --check scripts/webbe-validators-fixture.mjs
	node --check scripts/filetype-fixture.mjs
	node --check scripts/forms-fixture.mjs
	node --check scripts/collate-fixture.mjs
	node --check scripts/pdf-fixture.mjs
	go vet -tags=integration ./...

# Needs Node, libvips (CGO) and `npm ci --prefix tests/interop` for the Node fixtures.
test-unit:
	go test -race -count=1 ./...

# Differential check against Adonis web-be in an owned schema (docs/MIGRATION.md).
test-contract:
	node scripts/contract-auth.mjs
	node scripts/contract-public.mjs
	node scripts/contract-members.mjs
	node scripts/contract-forms.mjs
	node scripts/contract-courses.mjs
	node scripts/contract-routing.mjs

sqlc:
	sqlc generate
