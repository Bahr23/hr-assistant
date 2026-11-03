FROM golang:1.25-alpine AS build
WORKDIR /src/backend
RUN apk add --no-cache ca-certificates
COPY shared /src/shared
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cleanup-worker ./cmd/cleanup-worker

FROM alpine:3.21 AS runtime
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/cleanup-worker /app/cleanup-worker
COPY backend/migrations /app/migrations
EXPOSE 8080
ENTRYPOINT ["/app/api"]
