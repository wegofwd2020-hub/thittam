.PHONY: help \
        infra-up infra-down infra-up-full \
        db-init db-drop db-reset \
        migrate-all migrate-down seed \
        run-all run-web \
        test test-race test-cover test-integration \
        validate-verticals \
        lint build clean

# ── Database URL ───────────────────────────────────────────────────────────────
# Default: system PostgreSQL on port 5433 (Ubuntu installs here — data persists
# across reboots, no Docker volume wipe issues).
# Override: DB_URL=postgres://thittam:thittam_dev@localhost:5434/thittam?sslmode=disable make migrate-all
DB_URL ?= postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable

# ── Infrastructure compose files ───────────────────────────────────────────────
INFRA_FILE     := infra/local/docker-compose.infra.yml   # Redis + NATS + MinIO (no postgres)
INFRA_FULL     := infra/local/docker-compose.yml          # Redis + NATS + MinIO + Docker postgres

help:
	@echo ""
	@echo "Thittam — available targets:"
	@echo ""
	@echo "  Infrastructure (middleware only — DB lives on system postgres):"
	@echo "    make infra-up       Start Redis, NATS, MinIO via Docker"
	@echo "    make infra-down     Stop middleware containers"
	@echo "    make infra-up-full  Start all infra including Docker postgres (port 5434)"
	@echo ""
	@echo "  Local database (system PostgreSQL on port 5433):"
	@echo "    make db-init        Create thittam role + database on system postgres"
	@echo "    make db-drop        Drop thittam database from system postgres"
	@echo "    make db-reset       Fresh start: drop → init → migrate → seed"
	@echo ""
	@echo "  Migrations:"
	@echo "    make migrate-all    Run all DB migrations (dependency order)"
	@echo "    make migrate-down   Roll back all migrations"
	@echo "    make seed           Load XYZ_CBA demo seed data"
	@echo ""
	@echo "  Run:"
	@echo "    make run-all        Start all 9 backend services (requires tmuxinator)"
	@echo "    make run-web        Start Next.js frontend on :3000"
	@echo ""
	@echo "  Quality:"
	@echo "    make test                Unit tests"
	@echo "    make test-race           Unit tests with race detector"
	@echo "    make test-cover          Coverage report (opens in browser)"
	@echo "    make validate-verticals  Validate all vertical YAML configs"
	@echo "    make lint                golangci-lint"
	@echo ""
	@echo "  DB_URL currently: $(DB_URL)"
	@echo ""

# ── Infrastructure ────────────────────────────────────────────────────────────

infra-up:
	docker compose -f $(INFRA_FILE) up -d
	@echo "==> Redis :6380, NATS :4222, MinIO :9000 ready."

infra-down:
	docker compose -f $(INFRA_FILE) down

infra-up-full:
	docker compose -f $(INFRA_FULL) up -d
	@echo "==> Redis :6380, NATS :4222, MinIO :9000, Postgres :5434 ready."

# ── Local database (system PostgreSQL) ────────────────────────────────────────

db-init:
	@./scripts/local-db-init.sh

db-drop:
	@./scripts/local-db-drop.sh

db-reset: db-drop db-init migrate-all seed
	@echo "==> Database reset complete."

# ── Migrations ────────────────────────────────────────────────────────────────
# Run in dependency order: iam first (tenants/users tables referenced by others)

migrate-all:
	@echo "==> Running migrations..."
	migrate -path migrations/shared        -database "$(DB_URL)&x-migrations-table=schema_migrations_shared" up
	migrate -path migrations/iam           -database "$(DB_URL)&x-migrations-table=schema_migrations_iam" up
	migrate -path migrations/project       -database "$(DB_URL)&x-migrations-table=schema_migrations_project" up
	migrate -path migrations/budget        -database "$(DB_URL)&x-migrations-table=schema_migrations_budget" up
	migrate -path migrations/expense       -database "$(DB_URL)&x-migrations-table=schema_migrations_expense" up
	migrate -path migrations/ledger        -database "$(DB_URL)&x-migrations-table=schema_migrations_ledger" up
	migrate -path migrations/inventory     -database "$(DB_URL)&x-migrations-table=schema_migrations_inventory" up
	migrate -path migrations/document      -database "$(DB_URL)&x-migrations-table=schema_migrations_document" up
	migrate -path migrations/notifications -database "$(DB_URL)&x-migrations-table=schema_migrations_notifications" up
	migrate -path migrations/reporting     -database "$(DB_URL)&x-migrations-table=schema_migrations_reporting" up
	migrate -path migrations/audit         -database "$(DB_URL)&x-migrations-table=schema_migrations_audit" up
	@echo "==> All migrations complete."

migrate-down:
	@echo "==> Rolling back migrations (reverse order)..."
	migrate -path migrations/audit         -database "$(DB_URL)&x-migrations-table=schema_migrations_audit" down
	migrate -path migrations/shared        -database "$(DB_URL)&x-migrations-table=schema_migrations_shared" down
	migrate -path migrations/reporting      -database "$(DB_URL)" down
	migrate -path migrations/notifications  -database "$(DB_URL)" down
	migrate -path migrations/document       -database "$(DB_URL)" down
	migrate -path migrations/inventory      -database "$(DB_URL)" down
	migrate -path migrations/ledger         -database "$(DB_URL)" down
	migrate -path migrations/expense        -database "$(DB_URL)" down
	migrate -path migrations/budget         -database "$(DB_URL)" down
	migrate -path migrations/project        -database "$(DB_URL)" down
	migrate -path migrations/iam            -database "$(DB_URL)" down
	@echo "==> Rollback complete."

# ── Seed data ─────────────────────────────────────────────────────────────────

seed:
	@echo "==> Loading XYZ_CBA demo seed data..."
	@for f in seeds/demo/xyz-cba/*.sql; do \
		echo "  Loading $$f"; \
		psql "$(DB_URL)" -f "$$f" || exit 1; \
	done
	@echo "==> Seed complete."

# ── Services ──────────────────────────────────────────────────────────────────

run-all:
	tmuxinator start thittam

run-web:
	cd web && npm run dev

# ── Tests ─────────────────────────────────────────────────────────────────────

test:
	go test ./... -short

test-race:
	go test ./... -short -race

test-cover:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out

test-integration:
	go test ./... -tags=integration -race

# ── Vertical schema validation ────────────────────────────────────────────────
# Validates every *.yaml file in pkg/vertical/configs/ against the vertical
# schema rules (required fields, enum values, acyclic phase graph, etc.).
# Run automatically in CI and locally via: make validate-verticals
validate-verticals:
	go test -run TestValidateAllProductionVerticals ./pkg/vertical/...

# ── Code quality ──────────────────────────────────────────────────────────────

lint:
	golangci-lint run ./...

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	go build ./cmd/...

clean:
	rm -f coverage.out
	go clean ./...
