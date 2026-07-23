# Notifications Authorization (#139 slice G) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gate the six notifications config/send RPCs on `notifications:read`/`notifications:manage`, self-scope the two personal-inbox reads to the caller's own recipient id (D9), and wire notifications fail-closed against IAM.

**Architecture:** Order matters. Task 1 closes the D9 recipient-scoping gap end-to-end (guard-by-type: `recipientID` becomes a required parameter, one commit, `go vet ./...` gate) so gates land on handlers that already scope correctly. Task 2 adds the two permission strings across all three "halves" (migration 023 + `systemRoles` + both seed fixtures). Task 3 adds the perm field, the six gates, and fail-closed `cmd` wiring.

**Tech Stack:** Go 1.25, gRPC, pgx, `pkg/interceptor` (`ActorFromRequest`, `RequirePermission`, `PermissionChecker`), `pkg/iamclient` (`DialFromEnv`), golang-migrate, sqlc-adjacent hand-written pgx.

## Global Constraints

- `notification_log.recipient_id` is already `UUID NOT NULL` — the D9 fix is a predicate, **no migration**.
- Caller identity comes from the verified token via `interceptor.ActorFromRequest(ctx, "")` — **never** from the request body.
- Cross-recipient / cross-tenant id on a self-scoped read returns `ErrNotificationNotFound` → gRPC `NotFound`. No existence oracle.
- The three-halves rule: any permission string added to `systemRoles` MUST also land in migration `023` **and** both seed fixtures (`seeds/demo/xyz-cba/007_iam_roles.sql`, `seeds/template/new-tenant/001_tenant.sql`), or the seeds drift (#168).
- `migrations/iam` runs against the **public schema** (`make migrate-all`) AND against each `tenant_<uuid>` at `CreateTenant` — every backfill UPDATE MUST be idempotent (`NOT (... = ANY(permissions))`) and touch `is_system = true` only.
- Grant matrix: `notifications:read` → {super_admin, manager}; `notifications:manage` → {super_admin, manager}. No accountant tier.
- `NewHandler` gains a **required** `perm interceptor.PermissionChecker` param; `cmd/notifications` dials IAM and `log.Fatalf` if absent — the service refuses to start without a permission checker.
- The NATS dispatcher (`cmd/notifications/dispatcher.go`) calls the **Service** layer directly and is never gated — do not touch it.
- `UpdateNotificationLog` internal status write (`db/postgres.go:152`) is dispatcher-internal, keyed by row id — correct-unscoped, leave as-is.
- Grant matrix + self-scope + idempotency are proven only by `//go:build integration` tests in the real-Postgres CI job. `Migration Validate` is syntax-only against an empty DB. `go vet -tags=integration ./...` is the only local compile signal for integration files.
- **DB verification:** never run `docker compose … -v`/`down`/`up` against `infra/local/`. Use a disposable uniquely-named throwaway container or `pkg/testdb` (SKIPs without `THITTAM_TEST_DSN`). CI's real-Postgres job is the authoritative up/down gate.
- Coverage floor for notifications: ≥ 75% (CLAUDE.md).

---

## File Structure

- `services/notifications/repository.go` — `Repository` interface: add `recipientID` to Get/List.
- `services/notifications/service.go` — thread `recipientID` through Get/List.
- `services/notifications/db/postgres.go` — `AND recipient_id = $N` on Get/List SQL.
- `services/notifications/handler.go` — perm field + 6 gates (Task 3); `ActorFromRequest` self-scope on 2 reads (Task 1).
- `services/notifications/handler_test.go` — self-scope helper + tests (Task 1); perm stubs + grant tests (Task 3).
- `services/notifications/service_test.go` — `mockRepo` signatures + call sites.
- `cmd/notifications/main.go` — `iamclient.DialFromEnv`, fail-closed, pass perm (Task 3).
- `cmd/notifications/dispatcher_test.go` — `mockTemplateRepo` signatures (Task 1).
- `e2e/critical_path/notifications_test.go` — `notifRepo` double signatures (Task 1).
- `services/iam/service.go` — `systemRoles` super_admin + manager (Task 2).
- `migrations/iam/023_seed_notifications_permissions.{up,down}.sql` — backfill (Task 2).
- `seeds/demo/xyz-cba/007_iam_roles.sql`, `seeds/template/new-tenant/001_tenant.sql` — seed grants (Task 2).
- `tests/integration/notifications_authz_test.go` — self-scope + grant matrix + idempotency (Tasks 1/2/3).

---

## Task 1: Self-scope the personal inbox (D9)

**Files:**
- Modify: `services/notifications/repository.go:22-23`
- Modify: `services/notifications/service.go:184-201`
- Modify: `services/notifications/db/postgres.go:161-189` and `:191-...`
- Modify: `services/notifications/handler.go:167-200`
- Modify: `services/notifications/service_test.go:88-96,353-380,508-533` (mockRepo + call sites)
- Modify: `cmd/notifications/dispatcher_test.go:55-60` (mockTemplateRepo double)
- Modify: `e2e/critical_path/notifications_test.go:114-135` (notifRepo double)
- Modify: `services/notifications/handler_test.go` (add `callerCtxWithUser` + self-scope tests)

**Interfaces:**
- Consumes: `interceptor.ActorFromRequest(ctx, reqActorID string) (uuid.UUID, error)` — returns the token's `UserID`, `Unauthenticated` if none, `PermissionDenied` if a non-empty reqActorID differs.
- Produces (new signatures every double must match):
  - `Repository.GetNotification(ctx, tenantID, recipientID, id uuid.UUID) (*Notification, error)`
  - `Repository.ListNotifications(ctx, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error)`
  - `Service.GetNotification(ctx, tenantID, recipientID, id uuid.UUID) (*Notification, error)`
  - `Service.ListNotifications(ctx, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error)`

- [ ] **Step 1: Write the failing unit test (self-scope predicate reaches repo)**

Add to `services/notifications/handler_test.go`, after the `callerCtx` helper:

```go
// callerCtxWithUser is callerCtx with a caller-controlled UserID, so a test can
// assert the recipient predicate the handler derives from the token.
func callerCtxWithUser(tid, uid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uid,
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
}

func TestHandler_ListNotifications_SelfScoped(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	uid := uuid.New()
	var gotRecipient uuid.UUID
	h := newHandlerWithRepo(&mockRepo{
		listNotificationsFn: func(_ context.Context, _ , recipientID uuid.UUID, _, _ string, _, _ int) ([]Notification, error) {
			gotRecipient = recipientID
			return nil, nil
		},
	})
	_, err := h.ListNotifications(callerCtxWithUser(tid, uid), &notificationsv1.ListNotificationsRequest{TenantId: tid.String()})
	require.NoError(t, err)
	assert.Equal(t, uid, gotRecipient)
}

func TestHandler_GetNotification_SelfScoped(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	uid := uuid.New()
	var gotRecipient uuid.UUID
	h := newHandlerWithRepo(&mockRepo{
		getNotificationFn: func(_ context.Context, _, recipientID, id uuid.UUID) (*Notification, error) {
			gotRecipient = recipientID
			return &Notification{ID: id, TenantID: tid}, nil
		},
	})
	_, err := h.GetNotification(callerCtxWithUser(tid, uid), &notificationsv1.GetNotificationRequest{
		TenantId: tid.String(), Id: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Equal(t, uid, gotRecipient)
}
```

Note: `mockRepo.getNotificationFn`/`listNotificationsFn` field signatures change in Step 4 to match the new repo interface; update the existing success-test closures at the same time (they currently take `(ctx, tid, id)` / `(ctx, tid, channel, status, limit, offset)`).

- [ ] **Step 2: Run to verify it fails to compile**

Run: `go test ./services/notifications/ -run SelfScoped 2>&1 | head`
Expected: compile error — `mockRepo` does not implement the not-yet-widened interface / arg count mismatch.

- [ ] **Step 3: Widen the interface and thread `recipientID`**

`services/notifications/repository.go` — the two lines:

```go
	GetNotification(ctx context.Context, tenantID, recipientID, id uuid.UUID) (*Notification, error)
	ListNotifications(ctx context.Context, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error)
```

`services/notifications/service.go` (185, 194) — add the param and pass it through:

```go
func (s *Service) GetNotification(ctx context.Context, tenantID, recipientID, id uuid.UUID) (*Notification, error) {
	n, err := s.repo.GetNotification(ctx, tenantID, recipientID, id)
	// ... unchanged body ...
}

func (s *Service) ListNotifications(ctx context.Context, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error) {
	// ... existing limit-clamping unchanged ...
	notifications, err := s.repo.ListNotifications(ctx, tenantID, recipientID, channel, status, limit, offset)
	// ... unchanged ...
}
```

`services/notifications/db/postgres.go` — `GetNotification` (161): add the param, add the predicate, bind it:

```go
func (p *Postgres) GetNotification(ctx context.Context, tenantID, recipientID, id uuid.UUID) (*notifications.Notification, error) {
	const sql = `SELECT id, tenant_id, recipient_id, channel, event_type, subject,
		provider_msg_id, status, retry_count, error_message, sent_at, delivered_at, created_at
		FROM notification_log WHERE id = $1 AND tenant_id = $2 AND recipient_id = $3`

	var row NotificationLog
	err := p.db.QueryRow(ctx, sql, id, tenantID, recipientID).Scan(
		// ... unchanged scan targets ...
	)
	// ... unchanged: pgx.ErrNoRows -> ErrNotificationNotFound ...
}
```

`ListNotifications` (191): add the param, add the predicate as `$2`, renumber the rest:

```go
func (p *Postgres) ListNotifications(ctx context.Context, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]notifications.Notification, error) {
	const sql = `SELECT id, tenant_id, recipient_id, channel, event_type, subject,
		provider_msg_id, status, retry_count, error_message, sent_at, delivered_at, created_at
		FROM notification_log
		WHERE tenant_id = $1
		  AND recipient_id = $2
		  AND ($3 = '' OR channel = $3)
		  AND ($4 = '' OR status = $4)
		ORDER BY created_at DESC
		LIMIT $5 OFFSET $6`

	rows, err := p.db.Query(ctx, sql, tenantID, recipientID, channel, status, int32(limit), int32(offset))
	// ... unchanged scan loop ...
}
```

- [ ] **Step 4: Update handler reads to derive recipient from the token**

`services/notifications/handler.go` — `GetNotification` (167): after the `TenantFromRequest` block and before `uuid.Parse(req.GetId())`, add:

```go
	recipientID, err := interceptor.ActorFromRequest(ctx, "")
	if err != nil {
		return nil, err
	}
```

and change the service call to `h.svc.GetNotification(ctx, tenantID, recipientID, id)`.

`ListNotifications` (184): after the `TenantFromRequest` block, add the same `recipientID` derivation, and change the call to:

```go
	notifs, err := h.svc.ListNotifications(ctx, tenantID, recipientID, req.GetChannel(), req.GetStatus(), int(req.GetLimit()), int(req.GetOffset()))
```

- [ ] **Step 5: Update every mock/double to the new signatures**

`services/notifications/service_test.go` `mockRepo` (88, 94) — change field types and methods:

```go
	getNotificationFn   func(ctx context.Context, tenantID, recipientID, id uuid.UUID) (*Notification, error)
	listNotificationsFn func(ctx context.Context, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error)
```

```go
func (m *mockRepo) GetNotification(ctx context.Context, tenantID, recipientID, id uuid.UUID) (*Notification, error) {
	return m.getNotificationFn(ctx, tenantID, recipientID, id)
}
func (m *mockRepo) ListNotifications(ctx context.Context, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error) {
	return m.listNotificationsFn(ctx, tenantID, recipientID, channel, status, limit, offset)
}
```

Fix the `svc.ListNotifications`/`svc.GetNotification` call sites in `service_test.go` (353-380, 508-533) to pass a `recipientID` (`uuid.New()` is fine where the value is not asserted).

`cmd/notifications/dispatcher_test.go` `mockTemplateRepo` (55, 58):

```go
func (r *mockTemplateRepo) GetNotification(_ context.Context, _, _, _ uuid.UUID) (*notifications.Notification, error) {
	return nil, nil
}
func (r *mockTemplateRepo) ListNotifications(_ context.Context, _, _ uuid.UUID, _, _ string, _, _ int) ([]notifications.Notification, error) {
	return nil, nil
}
```

`e2e/critical_path/notifications_test.go` `notifRepo` (114, 125) — mirror the same widened signatures (add the `recipientID` param; keep the existing body, applying `recipient_id` filtering only if the double's stored data models it — otherwise leave the body's behavior and just satisfy the signature).

Also update the existing `TestHandler_GetNotification_Success` / `TestHandler_ListNotifications_Success` inline `mockRepo` closures to the new `func(... recipientID ...)` shape.

- [ ] **Step 6: Run unit tests + whole-tree vet**

Run: `go test ./services/notifications/... ./cmd/notifications/... && go vet ./...`
Expected: PASS; vet clean (this is the gate that catches the `e2e/critical_path` double — `go build ./services/...` alone will not).

- [ ] **Step 7: Write the failing integration test (self-scope across recipients)**

Create `tests/integration/notifications_authz_test.go`:

```go
//go:build integration

package integration
```

Add a test that: seeds two recipients (A, B) in one tenant with a `notification_log` row each; calls the repo/service `ListNotifications` as A and asserts only A's row returns; calls `GetNotification` for B's id as A and asserts `ErrNotificationNotFound`. Use `pkg/testdb` (SKIPs without `THITTAM_TEST_DSN`). Follow the existing integration-test harness in `tests/integration/` for tenant/schema setup. **DB safety: throwaway container or testdb only; never `docker compose -v` on infra/local.**

- [ ] **Step 8: Run integration test (skips locally without DSN; real gate is CI)**

Run: `go vet -tags=integration ./tests/integration/ && go test -tags=integration ./tests/integration/ -run Notifications 2>&1 | tail`
Expected: compiles; SKIP locally without `THITTAM_TEST_DSN` (CI real-Postgres job runs it).

- [ ] **Step 9: Commit**

```bash
git add services/notifications cmd/notifications/dispatcher_test.go e2e/critical_path/notifications_test.go tests/integration/notifications_authz_test.go
git commit -m "fix(notifications): self-scope the personal inbox to the caller (#139 D9)"
```

---

## Task 2: notifications vocabulary + three-halves backfill

**Files:**
- Create: `migrations/iam/023_seed_notifications_permissions.up.sql`
- Create: `migrations/iam/023_seed_notifications_permissions.down.sql`
- Modify: `services/iam/service.go:71-90` (`systemRoles` super_admin + manager)
- Modify: `seeds/demo/xyz-cba/007_iam_roles.sql`
- Modify: `seeds/template/new-tenant/001_tenant.sql`

**Interfaces:**
- Produces: permission strings `notifications:read`, `notifications:manage` granted to `super_admin`, `manager` across all three halves.

- [ ] **Step 1: Write migration 023 up**

Create `migrations/iam/023_seed_notifications_permissions.up.sql`:

```sql
-- 023_seed_notifications_permissions.up.sql
-- #139 slice G: grant the two notifications permissions to existing tenants.
--
-- systemRoles (services/iam/service.go) is edited in the same change for new
-- tenants; both seed fixtures too. All three halves required (see #168).
--
-- Idempotent by necessity: migrations/iam runs against the public schema via
-- `make migrate-all` AND against every new tenant_<uuid> at CreateTenant.
-- is_system = true only.

UPDATE roles SET permissions = array_append(permissions, 'notifications:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('notifications:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'notifications:manage')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('notifications:manage' = ANY (permissions));
```

- [ ] **Step 2: Write migration 023 down**

Create `migrations/iam/023_seed_notifications_permissions.down.sql`:

```sql
-- 023_seed_notifications_permissions.down.sql
-- Reverse of 023. Both strings are new to every role this migration touches,
-- so removing each unconditionally across is_system roles is correct.

UPDATE roles SET permissions = array_remove(permissions, 'notifications:read')   WHERE is_system = true;
UPDATE roles SET permissions = array_remove(permissions, 'notifications:manage') WHERE is_system = true;
```

- [ ] **Step 3: Edit `systemRoles` (super_admin + manager only)**

`services/iam/service.go` — in the `super_admin` block, after the `"document:read", "document:write", "document:delete",` line add:

```go
		"notifications:read", "notifications:manage",
```

In the `manager` block, after its `"document:read", "document:write", "document:delete",` line add the same line. Add to **no other role**.

- [ ] **Step 4: Edit both seed fixtures**

In `seeds/demo/xyz-cba/007_iam_roles.sql`, the two permission arrays that contain `'billing:read','billing:manage'` (super_admin and manager) — append `'notifications:read','notifications:manage'` to each.

In `seeds/template/new-tenant/001_tenant.sql`, do the same to the two arrays containing `'billing:read','billing:manage'`.

- [ ] **Step 5: Verify all three halves with literal counts**

Run:
```bash
grep -cF 'notifications:read' services/iam/service.go
grep -cF 'notifications:manage' services/iam/service.go
grep -cF 'notifications:read' seeds/demo/xyz-cba/007_iam_roles.sql
grep -cF 'notifications:read' seeds/template/new-tenant/001_tenant.sql
grep -cF 'notifications:read' migrations/iam/023_seed_notifications_permissions.up.sql
```
Expected: service.go → `2` each (super_admin + manager); each seed → `2` (super_admin + manager arrays); migration up → `1`. If any count is off, a half drifted — fix before proceeding.

- [ ] **Step 6: Add migration idempotency + grant-presence integration assertion**

Extend `tests/integration/notifications_authz_test.go` with a test that applies migrations to a fresh tenant schema (via the harness / `pkg/testdb`), runs `023` up twice, and asserts `super_admin` and `manager` each hold both strings exactly once; runs `down` and asserts both strings are gone. **DB safety as in Task 1.**

- [ ] **Step 7: Verify migration compiles/validates**

Run: `go vet -tags=integration ./tests/integration/`
Expected: clean. (Real up/down proof is the CI real-Postgres + Migration Validate jobs.)

- [ ] **Step 8: Commit**

```bash
git add migrations/iam/023_seed_notifications_permissions.up.sql migrations/iam/023_seed_notifications_permissions.down.sql services/iam/service.go seeds/demo/xyz-cba/007_iam_roles.sql seeds/template/new-tenant/001_tenant.sql tests/integration/notifications_authz_test.go
git commit -m "feat(iam): add notifications:read/manage vocabulary + backfill (#139 slice G)"
```

---

## Task 3: Gates + fail-closed wiring

**Files:**
- Modify: `services/notifications/handler.go:22-33` (perm field + `NewHandler`), `:37-163` (6 gates)
- Modify: `services/notifications/handler_test.go` (perm stubs; thread perm through `newHandler`/`newHandlerWithRepo` + inline constructions; grant-matrix tests)
- Modify: `cmd/notifications/main.go:12-16,78` (import iamclient, dial, fail-closed, pass perm)
- Modify: `tests/integration/notifications_authz_test.go` (grant-matrix)

**Interfaces:**
- Consumes: `interceptor.PermissionChecker` (`CheckPermission(ctx, userID, permission string, projectID *uuid.UUID) (bool, error)`), `interceptor.RequirePermission(ctx, checker, permission) error`, `iamclient.DialFromEnv(serviceName) (*iamclient.PermissionChecker, func() error, error)`, `iamclient.EnvAddr`.
- Produces: `NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler`.

- [ ] **Step 1: Write the failing grant-matrix unit tests**

In `services/notifications/handler_test.go`, add perm doubles (copy the billing/document pattern):

```go
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return true, nil
}

type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}
```

Add tests asserting a denied checker blocks the six gated RPCs and does NOT block the two self-scoped reads:

```go
func TestHandler_Templates_Denied(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{}, map[string]ChannelSender{}), denyPerm{})
	_, err := h.ListTemplates(callerCtx(tid), &notificationsv1.ListTemplatesRequest{TenantId: tid.String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = h.Send(callerCtx(tid), &notificationsv1.SendRequest{TenantId: tid.String(), RecipientId: uuid.New().String(), Channel: "email", EventType: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_Inbox_NotGated(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listNotificationsFn: func(context.Context, uuid.UUID, uuid.UUID, string, string, int, int) ([]Notification, error) { return nil, nil },
	}, map[string]ChannelSender{}), denyPerm{})
	_, err := h.ListNotifications(callerCtx(tid), &notificationsv1.ListNotificationsRequest{TenantId: tid.String()})
	require.NoError(t, err) // denyPerm must not block a self-scoped read
}
```

- [ ] **Step 2: Run to verify failure (compile: NewHandler arity)**

Run: `go test ./services/notifications/ -run 'Templates_Denied|Inbox_NotGated' 2>&1 | head`
Expected: compile error — `NewHandler` takes 1 arg, not 2.

- [ ] **Step 3: Add the perm field, required constructor, and six gates**

`services/notifications/handler.go` — struct + constructor:

```go
type Handler struct {
	notificationsv1.UnimplementedNotificationsServiceServer
	svc  *Service
	perm interceptor.PermissionChecker
}

// NewHandler creates a Handler. perm is required; cmd/notifications refuses to
// start without a permission checker, so it is never nil in production.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}
```

Insert the gate as the first statement of each of the six handlers (before `TenantFromRequest`):

- `CreateTemplate`, `UpdateTemplate`, `Send`, `Dispatch`:
```go
	if err := interceptor.RequirePermission(ctx, h.perm, "notifications:manage"); err != nil {
		return nil, err
	}
```
- `GetTemplate`, `ListTemplates`:
```go
	if err := interceptor.RequirePermission(ctx, h.perm, "notifications:read"); err != nil {
		return nil, err
	}
```

Leave `Send`/`Dispatch` bodies (recipient parsing, service calls) otherwise unchanged. Do NOT add a gate to `GetNotification`/`ListNotifications` — they are self-scoped AUTH.

- [ ] **Step 4: Thread perm through the test helpers**

`services/notifications/handler_test.go` — update the two helpers and the inline constructions:

```go
func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{}, map[string]ChannelSender{}), allowAllPerm{})
}
func newHandlerWithRepo(r *mockRepo) *Handler {
	return NewHandler(NewService(r, map[string]ChannelSender{}), allowAllPerm{})
}
```

Update the inline `NewHandler(NewService(...))` calls in `TestHandler_Send_Success`, `TestHandler_GetNotification_Success`, `TestHandler_ListNotifications_Success` (and any other inline construction) to pass `allowAllPerm{}` — except the two new denyPerm tests, which pass `denyPerm{}`.

- [ ] **Step 5: Wire cmd/notifications fail-closed**

`cmd/notifications/main.go` — add `"github.com/wegofwd2020/thittam/pkg/iamclient"` to imports. Replace `handler := notifications.NewHandler(svc)` (line 78) with the billing pattern:

```go
	iamPerm, closeIAM, err := iamclient.DialFromEnv("notifications")
	if err != nil {
		log.Fatalf("notifications: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()
	if iamPerm == nil {
		log.Fatalf("notifications: startup: %s is not set; notifications cannot authorize without a permission checker", iamclient.EnvAddr)
	}

	handler := notifications.NewHandler(svc, iamPerm)
```

- [ ] **Step 6: Run unit tests + whole-tree vet**

Run: `go test ./services/notifications/... ./cmd/notifications/... && go vet ./...`
Expected: PASS; vet clean.

- [ ] **Step 7: Extend the integration test with the grant matrix**

In `tests/integration/notifications_authz_test.go`, assert against real IAM/Postgres: a role holding neither string → `PermissionDenied` on `GetTemplate`/`ListTemplates`/`CreateTemplate`/`UpdateTemplate`/`Send`/`Dispatch`; `super_admin`/`manager` pass all six; the two inbox reads succeed for any authenticated member regardless of `notifications:*`. **DB safety as in Task 1.**

- [ ] **Step 8: Run integration compile check**

Run: `go vet -tags=integration ./tests/integration/`
Expected: clean (CI real-Postgres job runs the assertions).

- [ ] **Step 9: Commit**

```bash
git add services/notifications cmd/notifications/main.go tests/integration/notifications_authz_test.go
git commit -m "feat(notifications): gate 6 RPCs on notifications:read/manage, wire fail-closed (#139 slice G)"
```

---

## Self-Review

- **Spec coverage:** self-scope D9 (Task 1) ✓; vocabulary + three-halves backfill (Task 2) ✓; six gates + fail-closed wiring (Task 3) ✓; dispatcher/`:152` untouched (Global Constraints) ✓; integration self-scope + grant matrix + idempotency (Tasks 1/2/3) ✓.
- **Placeholder scan:** every code step carries concrete code; test bodies are spelled out; the only "follow the existing harness" defers are the tenant/schema setup in `tests/integration/`, which is an existing pattern, not new logic.
- **Type consistency:** `GetNotification(ctx, tenantID, recipientID, id)` and `ListNotifications(ctx, tenantID, recipientID, channel, status, limit, offset)` are used identically across interface, service, postgres, and all three doubles; `NewHandler(svc, perm)` used identically in cmd + tests; `notifications:read`/`notifications:manage` identical across systemRoles, migration, both seeds.
