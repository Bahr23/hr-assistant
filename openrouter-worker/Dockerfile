FROM golang:1.25-alpine AS build
WORKDIR /src/openrouter-worker
RUN apk add --no-cache ca-certificates
COPY shared /src/shared
COPY openrouter-worker/go.mod openrouter-worker/go.sum* ./
RUN go mod download
COPY openrouter-worker .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/openrouter-worker ./cmd/worker

FROM alpine:3.21 AS runtime
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/openrouter-worker /app/openrouter-worker
COPY openrouter-worker/migrations /app/migrations
EXPOSE 8081
ENTRYPOINT ["/app/openrouter-worker"]
