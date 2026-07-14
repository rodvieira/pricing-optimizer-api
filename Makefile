# Backend developer tasks. Run `make help` for the list.

GO           ?= go
GOOSE        ?= goose
MIGRATIONS   := db/migrations
GOOSE_DRIVER ?= postgres
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/pricing?sslmode=disable

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: generate
generate: ## Regenerate code from the OpenAPI contract
	$(GO) generate ./...

.PHONY: fmt
fmt: ## Format Go code
	golangci-lint fmt

.PHONY: lint
lint: ## Run the linters
	golangci-lint run

.PHONY: test
test: ## Run unit tests
	$(GO) test ./... -short

.PHONY: test-race
test-race: ## Run all tests with the race detector (needs a C toolchain)
	$(GO) test ./... -race

.PHONY: build
build: ## Build the API binary into bin/
	$(GO) build -o bin/api ./cmd/api

.PHONY: run
run: ## Run the API locally
	$(GO) run ./cmd/api

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	$(GOOSE) -dir $(MIGRATIONS) $(GOOSE_DRIVER) "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back the most recent migration
	$(GOOSE) -dir $(MIGRATIONS) $(GOOSE_DRIVER) "$(DATABASE_URL)" down

.PHONY: migrate-status
migrate-status: ## Show migration status
	$(GOOSE) -dir $(MIGRATIONS) $(GOOSE_DRIVER) "$(DATABASE_URL)" status

.PHONY: migrate-create
migrate-create: ## Create a new SQL migration (make migrate-create name=add_x)
	@test -n "$(name)" || { echo "usage: make migrate-create name=<migration_name>"; exit 1; }
	$(GOOSE) -dir $(MIGRATIONS) create $(name) sql
