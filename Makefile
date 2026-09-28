# Lumen dev commands. Run `make` or `make help` to list them.
# Secrets come from api/.env (copy api/.env.example first).

SHELL := /bin/bash
.DEFAULT_GOAL := help

API_DIR := api
WEB_DIR := web
ENV     := $(API_DIR)/.env
LOAD    := set -a; [ -f $(CURDIR)/$(ENV) ] && . $(CURDIR)/$(ENV); set +a;
LOCAL_DB := postgres://lumen:lumen@localhost:5432/lumen?sslmode=disable

.PHONY: help setup env db-up db-down db-reset admin api worker web dev \
        seed seed-remove test test-db typecheck build docker-api docker-worker fmt tidy clean check-env

help: ## Show this help
	@awk 'BEGIN{FS=":.*## "; printf "\nUsage: make <target>\n\n"} /^[a-zA-Z_-]+:.*## /{printf "  \033[33m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo

# ---------- first-time setup ----------

setup: env db-up ## One-time: env files, local Postgres, Go + npm deps
	cd $(API_DIR) && go mod download
	cd $(WEB_DIR) && npm install
	@echo "\nNext: fill in $(ENV), then run: make admin EMAIL=you@example.com && make dev"

env: ## Create api/.env and web/.env.local from the examples (never overwrites)
	@[ -f $(ENV) ] || { cp $(API_DIR)/.env.example $(ENV); \
	  sed -i.bak "s|^SESSION_SECRET=.*|SESSION_SECRET=$$(openssl rand -hex 32)|" $(ENV) && rm -f $(ENV).bak; \
	  echo "created $(ENV) (SESSION_SECRET generated; add TMDB + R2 keys)"; }
	@[ -f $(WEB_DIR)/.env.local ] || { cp $(WEB_DIR)/.env.example $(WEB_DIR)/.env.local; echo "created $(WEB_DIR)/.env.local"; }

check-env:
	@[ -f $(ENV) ] || { echo "Missing $(ENV). Run: make env"; exit 1; }

# ---------- database ----------

db-up: ## Start local Postgres (docker compose)
	docker compose up -d postgres
	@until docker compose exec -T postgres pg_isready -U lumen >/dev/null 2>&1; do sleep 1; done; echo "postgres ready"

db-down: ## Stop local Postgres (data kept)
	docker compose down

db-reset: ## Wipe local Postgres data
	docker compose down -v

# ---------- run ----------

admin: check-env ## Create/reset an admin: make admin EMAIL=you@example.com
	@[ -n "$(EMAIL)" ] || { echo "Usage: make admin EMAIL=you@example.com"; exit 1; }
	$(LOAD) cd $(API_DIR) && go run ./cmd/admin -email "$(EMAIL)"

api: check-env ## Run the Go API on :8080
	$(LOAD) cd $(API_DIR) && go run ./cmd/api

worker: check-env ## Run the encode worker (needs ffmpeg)
	@command -v ffmpeg >/dev/null || { echo "Install ffmpeg first (brew install ffmpeg / apt install ffmpeg)"; exit 1; }
	$(LOAD) cd $(API_DIR) && go run ./cmd/worker

web: ## Run the Next.js dev server on :3000
	cd $(WEB_DIR) && npm run dev

dev: check-env ## Run API + worker + web together (Ctrl+C stops all)
	@$(MAKE) --no-print-directory -j3 api worker web

seed: check-env ## Add demo public-domain films (no video) to preview the home page
	$(LOAD) cd $(API_DIR) && go run ./cmd/seed

seed-remove: check-env ## Delete the demo films added by make seed
	$(LOAD) cd $(API_DIR) && go run ./cmd/seed -remove

# ---------- quality ----------

test: ## Go unit + encoder tests
	cd $(API_DIR) && go vet ./... && go test ./...

test-db: ## Go tests including DB/auth tests against local Postgres
	cd $(API_DIR) && TEST_DATABASE_URL="$(LOCAL_DB)" go test -count=1 ./...

typecheck: ## TypeScript check for the web app
	cd $(WEB_DIR) && npx tsc --noEmit

fmt: ## Format Go code
	cd $(API_DIR) && gofmt -w .

tidy: ## Tidy Go modules
	cd $(API_DIR) && go mod tidy

build: ## Build Go binaries into api/bin and the Next.js production bundle
	cd $(API_DIR) && CGO_ENABLED=0 go build -o bin/ ./cmd/...
	cd $(WEB_DIR) && npm run build

docker-api: ## Build the API image
	docker build -t lumen-api -f $(API_DIR)/Dockerfile $(API_DIR)

docker-worker: ## Build the worker image (ffmpeg included)
	docker build -t lumen-worker -f $(API_DIR)/Dockerfile.worker $(API_DIR)

clean: ## Remove build output
	rm -rf $(API_DIR)/bin $(WEB_DIR)/.next
