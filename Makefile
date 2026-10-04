.PHONY: build test vet fmt fmt-check check
build:
	mkdir -p bin
	go build -trimpath -o bin/spuro ./cmd/spuro
	go build -trimpath -o bin/spuro-plugin-gitwell ./cmd/spuro-plugin-gitwell
	go build -trimpath -o bin/spuro-plugin-stalewood ./cmd/spuro-plugin-stalewood

test:
	go test ./...
vet:
	go vet ./...
fmt:
	go fmt ./...
fmt-check:
	test -z "$$(gofmt -l .)"
check: fmt-check vet test
	go test -race ./...
