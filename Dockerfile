FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o harvester ./cmd/harvester

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/harvester /usr/local/bin/harvester
CMD ["harvester"]
