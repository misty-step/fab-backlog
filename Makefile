.PHONY: build test lint clean

BINARY_NAME=fab-backlog

build:
	go build -o $(BINARY_NAME) .

test:
	go test -v ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY_NAME)
