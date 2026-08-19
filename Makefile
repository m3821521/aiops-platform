.PHONY: tidy fmt vet test run docker

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

run:
	go run ./cmd/server

docker:
	docker build -t aiops-platform:0.1.0 .
