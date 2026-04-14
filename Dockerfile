FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/port-server ./cmd/port-server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/bin/port-server /usr/local/bin/port-server
ENTRYPOINT ["port-server"]
