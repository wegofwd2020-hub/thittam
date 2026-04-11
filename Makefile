.PHONY: help \
        infra-up infra-down infra-up-full \
        nats-provision \
        db-init db-drop db-reset \
        migrate-all migrate-down migrate-tenant migrate-all-tenants seed \
        run-all run-web \
        test test-race test-cover test-integration \
        validate-verticals coverage-check \
        generate generate-proto generate-sqlc \
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
	@echo "    make nats-provision Provision JetStream streams and consumers (requires nats CLI)"
	@echo ""
	@echo "  Local database (system PostgreSQL on port 5433):"
	@echo "    make db-init        Create thittam role + database on system postgres"
	@echo "    make db-drop        Drop thittam database from system postgres"
	@echo "    make db-reset       Fresh start: drop → init → migrate → seed"
	@echo ""
	@echo "  Migrations:"
	@echo "    make migrate-all               Run all DB migrations (dependency order)"
	@echo "    make migrate-down              Roll back all migrations"
	@echo "    make migrate-tenant id=<uuid>  Migrate a single tenant schema"
	@echo "    make migrate-all-tenants       Parallel migration runner for all tenants"
	@echo "    make seed                      Load XYZ_CBA demo seed data"
	@echo ""
	@echo "  Run:"
	@echo "    make run-all        Start all 9 backend services (requires tmuxinator)"
	@echo "    make run-web        Start Next.js frontend on :3000"
	@echo ""
	@echo "  Quality:"
	@echo "    make test                Unit tests"
	@echo "    make test-race           Unit tests with race detector"
	@echo "    make test-cover          Coverage report (opens in browser)"
	@echo "    make coverage-check      Enforce per-package coverage thresholds (CI parity)"
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

nats-provision:
	@./infra/nats/provision.sh

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

# migrate-tenant runs all service migrations for a single tenant schema.
# Usage: make migrate-tenant id=<uuid>  e.g. make migrate-tenant id=a1b2c3d4-...
# Useful when onboarding a new tenant or re-running a failed per-tenant migration.
migrate-tenant:
	@test -n "$(id)" || (echo "ERROR: id is required. Usage: make migrate-tenant id=<uuid>" && exit 1)
	@echo "==> Migrating tenant schema: tenant_$(id)"
	@for path in iam project budget expense ledger inventory document notifications reporting audit; do \
		migrate \
			-path "migrations/$$path" \
			-database "$(DB_URL)&search_path=tenant_$(id),public&x-migrations-table=schema_migrations_$$path" \
			up; \
	done
	@echo "==> Tenant $(id) migration complete."

# migrate-all-tenants runs the parallel migration runner against every tenant.
# Reads tenant IDs from the DB, runs up to 8 concurrent migrations per service.
# Usage: make migrate-all-tenants [path=migrations/project] [parallelism=8]
# On partial failure the runner prints failed IDs — re-run with migrate-tenant.
MIGRATE_PATH        ?= migrations/project
MIGRATE_PARALLELISM ?= 8
migrate-all-tenants:
	@echo "==> Running parallel tenant migrations: path=$(MIGRATE_PATH) workers=$(MIGRATE_PARALLELISM)"
	go run ./cmd/migrate-all-tenants \
		-db "$(DB_URL)" \
		-path "$(MIGRATE_PATH)" \
		-parallelism "$(MIGRATE_PARALLELISM)"

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

# ── Coverage enforcement ───────────────────────────────────────────────────────
# Mirrors the CI thresholds: iam/ledger ≥85%, budget/expense ≥80%, others ≥75%
# Usage: make coverage-check

coverage-check:
	@echo "==> Running coverage checks..."
	@$(MAKE) _cov-enforce PKG=services/iam           MIN=85
	@$(MAKE) _cov-enforce PKG=services/ledger        MIN=85
	@$(MAKE) _cov-enforce PKG=services/budget        MIN=80
	@$(MAKE) _cov-enforce PKG=services/expense       MIN=80
	@$(MAKE) _cov-enforce PKG=services/document      MIN=75
	@$(MAKE) _cov-enforce PKG=services/inventory     MIN=75
	@$(MAKE) _cov-enforce PKG=services/reporting     MIN=75
	@$(MAKE) _cov-enforce PKG=services/project       MIN=75
	@$(MAKE) _cov-enforce PKG=services/notifications MIN=75
	@echo "==> All coverage thresholds passed."

_cov-enforce:
	@go test ./$(PKG)/... -short -coverprofile=/tmp/cov_check.out 2>/dev/null
	@PCT=$$(go tool cover -func=/tmp/cov_check.out | grep "^total:" | awk '{print $$3}' | tr -d '%'); \
	 echo "  $(PKG): $${PCT}% (threshold: $(MIN)%)"; \
	 if ! echo "$${PCT} >= $(MIN)" | bc -l | grep -q '^1$$'; then \
	   echo "FAIL: $(PKG) coverage $${PCT}% is below $(MIN)%"; \
	   exit 1; \
	 fi

# ── Code generation ───────────────────────────────────────────────────────────

generate: generate-proto generate-sqlc

generate-proto:
	buf generate proto

generate-sqlc:
	sqlc generate

# ── Code quality ──────────────────────────────────────────────────────────────

lint:
	golangci-lint run ./...

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	go build ./cmd/...

clean:
	rm -f coverage.out
	go clean ./...
