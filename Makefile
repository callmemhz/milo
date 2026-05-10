.PHONY: build test test-integration lint generate run-server clean

build:
	go build -o bin/milod ./cmd/milod
	go build -o bin/milo ./cmd/milo

test:
	go test ./...

test-integration:
	go test -tags=docker_integration ./...

lint:
	go vet ./...
	gofmt -l . | tee /tmp/gofmt-out && test ! -s /tmp/gofmt-out

generate:
	sqlc generate

run-server:
	go run ./cmd/milod

clean:
	rm -rf bin/
