GO ?= go

.PHONY: test test-short bench bench-dht vet lint fmt build build-node build-dhtsim docker-dev docker-monitoring docker-down

test:
	$(GO) test ./...

test-short:
	$(GO) test -short ./...

bench:
	$(GO) test ./... -bench=. -benchmem

bench-dht:
	$(GO) test ./pkg/dht -bench=. -benchmem

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w cmd pkg

build: build-node build-dhtsim

build-node:
	$(GO) build -o bin/node ./cmd/node

build-dhtsim:
	$(GO) build -o bin/dhtsim ./cmd/dhtsim

docker-dev:
	docker compose -f deployments/compose/docker-compose.dev.yml up --build

docker-monitoring:
	docker compose -f deployments/compose/docker-compose.monitoring.yml up --build

docker-down:
	docker compose -f deployments/compose/docker-compose.dev.yml down
