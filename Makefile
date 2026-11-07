-include .env

export

.PHONY: generate generate-api generate-mocks migrate migrate-down run dev test test-unit test-integration lint lint-fix

generate: generate-api generate-mocks

generate-api:
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

generate-mocks:
	go generate ./internal/...

migrate:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" down

run:
	go run ./cmd/trip-service

dev: migrate
	$(MAKE) run

test:
	go test -race -cover ./...

test-unit:
	go test -race -cover ./...

test-integration:
	go test -race -count=2 -tags=integration ./...

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint fmt ./...
	golangci-lint run --fix ./...