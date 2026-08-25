.PHONY: run build test fmt

run:
	go run ./cmd/stormflix

build:
	go build -trimpath -o ./stormflix ./cmd/stormflix

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal
