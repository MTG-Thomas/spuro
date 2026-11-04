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

.PHONY: secret-scan pre-push
# Local authenticated gate; keep CI/check/test targets unchanged.
secret-scan: secret-gate-python
	"$(SECRET_GATE_PYTHON)" -I scripts/secret_gate.py

pre-push: secret-gate-python
	"$(SECRET_GATE_PYTHON)" -I scripts/secret_gate.py --pre-push

# Native Windows: set MTG_SECRET_GATE_PYTHON to an approved absolute interpreter.
PYTHON ?= python3

.PHONY: secret-gate-python
ifeq ($(OS),Windows_NT)
SECRET_GATE_PYTHON ?= $(MTG_SECRET_GATE_PYTHON)
else
SECRET_GATE_PYTHON ?= $(PYTHON)
endif

secret-gate-python:
ifeq ($(OS),Windows_NT)
	$(if $(or $(findstring :/,$(subst \,/,$(SECRET_GATE_PYTHON))),$(filter //%,$(subst \,/,$(SECRET_GATE_PYTHON)))),,$(error UNAVAILABLE: set MTG_SECRET_GATE_PYTHON to an approved absolute Python path outside all Git checkouts; publication blocked))
endif
