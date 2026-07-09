FROM golang:1.26-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app app \
    && mkdir -p /app/uploads \
    && chown -R app:app /app

COPY --from=builder /out/server /app/server
COPY configs /app/configs
COPY script /app/script

USER app

EXPOSE 8081
ENTRYPOINT ["/app/server"]
CMD ["--config", "/app/configs/config.docker.yaml"]