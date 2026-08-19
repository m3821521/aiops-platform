# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/aiops-server ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -u 10001 aiops
COPY --from=build /out/aiops-server /app/aiops-server
COPY docs/swagger.json /app/docs/swagger.json
COPY configs/config.example.yaml /app/configs/config.example.yaml
USER aiops
EXPOSE 8080
ENTRYPOINT ["/app/aiops-server"]
