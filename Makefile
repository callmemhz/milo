.PHONY: build test test-integration lint generate run-server clean

build:
	go build -o bin/milo-apps-kit-server ./cmd/milo-apps-kit-server
	go build -o bin/milo-apps-kit ./cmd/milo-apps-kit

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
	go run ./cmd/milo-apps-kit-server

clean:
	rm -rf bin/
