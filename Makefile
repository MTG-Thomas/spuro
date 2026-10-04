.PHONY: build test check
build:
	mkdir -p bin
	go build -o bin/spuro ./cmd/spuro
	go build -o bin/spuro-plugin-gitwell ./cmd/spuro-plugin-gitwell
	go build -o bin/spuro-plugin-stalewood ./cmd/spuro-plugin-stalewood

test:
	go test ./...

check:
	go test -race ./...
	go vet ./...
