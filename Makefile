.PHONY: build test vet fmt fmt-check release-test check
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
release-test:
	python3 -m unittest discover -s scripts -p 'test_release.py' -v

check: fmt-check vet test release-test
	go test -race ./...
