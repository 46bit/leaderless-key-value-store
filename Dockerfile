ARG go_version=1.17-alpine
FROM golang:$go_version AS build
WORKDIR /app
COPY $PWD/go.mod /app
COPY $PWD/go.sum /app
RUN go mod download
RUN apk add --no-cache make
COPY $PWD /app
RUN make build

FROM alpine:latest AS run
WORKDIR /app
COPY --from=build /app/bin /app/bin
COPY k8s-storage-node-startup.sh /app/bin
RUN apk add --no-cache gettext
