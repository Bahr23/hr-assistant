.PHONY: generate test run

generate:
	docker run --rm -v "$(PWD):/src" -w /src sqlc/sqlc:1.28.0 generate
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config oapi-auth.yaml openapi/openapi.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config oapi-vacancy.yaml openapi/openapi.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config oapi-candidate.yaml openapi/openapi.yaml

test:
	go test ./...

run:
	go run ./cmd/api
