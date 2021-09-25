.PHONY: all
all: generate fmt vet build

.PHONY: generate
generate:
	protoc -I=. --go_out=. --go-grpc_out=. api/api.proto

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: build
build:
	mkdir -p bin
	go build -o ./bin/coordinator ./cmd/coordinator
	go build -o ./bin/storage_node ./cmd/storage_node

.PHONY: docker
docker: docker-build docker-push

.PHONY: docker-build
docker-build:
	docker build -t ghcr.io/46bit/leaderless-key-value-store .

.PHONY: docker-push
docker-push:
	docker push ghcr.io/46bit/leaderless-key-value-store
