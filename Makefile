.PHONY: help \
        infra-up infra-down infra-up-full \
        nats-provision \
        db-init db-drop db-reset db-bootstrap \
        db-test-bootstrap db-test-reset db-grant-app-role \
        migrate-all migrate-down migrate-tenant migrate-all-tenants seed seed-construction \
        dev-start dev-start-fresh dev-stop \
        run-all run-web \
        test test-race test-cover test-integration test-e2e test-e2e-install \
        validate-verticals coverage-check \
        generate generate-proto generate-sqlc \
        lint build clean \
        dev-keys

# ── Database URL ───────────────────────────────────────────────────────────────
# Default: system PostgreSQL on port 5433 (Ubuntu installs here — data persists
# across reboots, no Docker volume wipe issues).
# Override: DB_URL=postgres://thittam:thittam_dev@localhost:5434/thittam?sslmode=disable make migrate-all
DB_URL ?= postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable

# Test database — fully isolated from dev DB. Owned by db-test-bootstrap /
# db-test-reset and read by `go test ... -tags=integration` via THITTAM_TEST_DSN.
TEST_DB_URL ?= postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable

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
	@echo "    make db-bootstrap            Idempotent: ensure DB exists + migrate (no data loss)"
	@echo "    make db-bootstrap WITH_SEED=1   Same, but also load XYZ_CBA seed"
	@echo "    make db-reset                Destructive: drop → init → migrate → seed"
	@echo "    make db-init                 (low-level) Create thittam role + database"
	@echo "    make db-drop                 (low-level) Drop thittam database"
	@echo ""
	@echo "  Test database (thittam_test — isolated from dev DB):"
	@echo "    make db-test-bootstrap       Idempotent: ensure thittam_test exists + migrate"
	@echo "    make db-test-reset           Destructive: drop → init → migrate"
	@echo "    make test-integration        Verify head, run -tags=integration tests"
	@echo ""
	@echo "  Migrations:"
	@echo "    make migrate-all               Run all DB migrations (dependency order)"
	@echo "    make migrate-down              Roll back all migrations"
	@echo "    make migrate-tenant id=<uuid>  Migrate a single tenant schema"
	@echo "    make migrate-all-tenants       Parallel migration runner for all tenants"
	@echo "    make seed                      Load XYZ_CBA demo seed data"
	@echo ""
	@echo "  Run:"
	@echo "    make dev-start         Start infra + services. Verifies DB head; never mutates DB."
	@echo "    make dev-start-fresh   db-reset → dev-start (clean slate, full rebuild)"
	@echo "    make dev-stop       Stop all dev services (keeps Docker containers running)"
	@echo "    make dev-stop-all   Stop all dev services AND Docker containers"
	@echo "    make run-all        Start all 9 backend services via tmuxinator (requires tmuxinator)"
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
	@echo "  Local dev keys (gitignored — never committed):"
	@echo "    make dev-keys       Generate keys/jwt_private.pem and keys/oidc_encryption.key (skips existing)"
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

# db-bootstrap — idempotent. Pass WITH_SEED=1 to also seed.
# This is what developers run after pulling new migrations; safe to re-run.
db-bootstrap:
	@if [ "$(WITH_SEED)" = "1" ]; then \
		./scripts/db-bootstrap.sh --with-seed; \
	else \
		./scripts/db-bootstrap.sh; \
	fi

# db-reset — destructive. Drop dev DB, recreate, migrate, seed.
db-reset:
	@./scripts/db-reset.sh

# db-test-bootstrap — idempotent. Ensure thittam_test exists and is at head.
db-test-bootstrap:
	@./scripts/db-test-bootstrap.sh

# db-grant-app-role — grant thittam_app least privilege + revoke UPDATE/DELETE on
# audit_log (#120). Run as owner (thittam) after migrate-all. Role must already
# exist (created by local-db-init.sh / CI provisioning — CREATE ROLE needs superuser).
db-grant-app-role:
	psql "$(DB_URL)" -v ON_ERROR_STOP=1 -f scripts/db-grant-app-role.sql

# db-test-reset — destructive. Drop and rebuild thittam_test only.
db-test-reset:
	@./scripts/db-test-reset.sh

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

# seed-construction — loads the XYZ Construction demo tenant. Independent of
# `make seed` so either tenant's fixtures can be reloaded in isolation.
seed-construction:
	@echo "==> Loading XYZ Construction demo seed data..."
	@for f in seeds/demo/xyz-construction/*.sql; do \
		echo "  Loading $$f"; \
		psql "$(DB_URL)" -f "$$f" || exit 1; \
	done
	@echo "==> Seed complete."

# ── Dev stack shortcuts ───────────────────────────────────────────────────────

dev-start:
	@./scripts/dev-start.sh

# dev-start-fresh — destructive convenience: rebuild dev DB then start.
# Equivalent to: make db-reset && make dev-start.
dev-start-fresh: db-reset dev-start

dev-stop:
	@./scripts/dev-stop.sh

dev-stop-all:
	@./scripts/dev-stop.sh --infra

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

# test-integration — bootstrap thittam_test (idempotent, no data loss) then
# run all integration-tagged tests. Honours THITTAM_TEST_DSN if exported.
test-integration: db-test-bootstrap
	THITTAM_TEST_DSN="$(TEST_DB_URL)" go test ./... -tags=integration -race

# test-e2e — Playwright UX tests against the Next.js frontend.
# Assumes backend services are up (make dev-start) and DB is seeded (make seed).
# Playwright auto-boots `npm run dev` on :3100 via its webServer config.
test-e2e:
	cd web && npm run test:e2e

test-e2e-install:
	cd web && npm install && npx playwright install --with-deps

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
	@$(MAKE) _cov-enforce PKG=services/billing      MIN=75
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

# ── Local dev key generation ──────────────────────────────────────────────────
# Creates gitignored key files for local development.
# Production keys come from Vault (T1 secrets — see CODING_RULES.md Rule #2).
# Never commit these files — they are listed in .gitignore.

dev-keys:
	@mkdir -p keys
	@touch keys/.gitkeep
	@if [ -f keys/jwt_private.pem ]; then \
		echo "  keys/jwt_private.pem already exists — skipping"; \
	else \
		openssl genrsa -out keys/jwt_private.pem 2048; \
		echo "  keys/jwt_private.pem generated (RSA-2048)"; \
	fi
	@if [ -f keys/oidc_encryption.key ]; then \
		echo "  keys/oidc_encryption.key already exists — skipping"; \
	else \
		openssl rand -out keys/oidc_encryption.key 32; \
		echo "  keys/oidc_encryption.key generated (32 random bytes, AES-256)"; \
	fi
	@echo "==> Dev keys ready in keys/  (gitignored — never commit these)"
