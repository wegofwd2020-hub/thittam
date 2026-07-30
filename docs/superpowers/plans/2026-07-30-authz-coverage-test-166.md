# Permission-gate coverage test (#166) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A source-parsing Go test that makes it impossible to merge a permission gate lacking its grant, or a newly-gated permission lacking an existing-tenant backfill migration — plus a short deploy-ordering ops note.

**Architecture:** One in-`package iam` test reads the unexported `systemRoles` directly and scans `services/` source (go/ast, resolving same-package string consts) and `migrations/iam/*.up.sql` (regex) to enforce three set-inclusion invariants. It rides the existing `go test ./...` (Test & Coverage CI job) — no ci.yml/k8s/DB change. A markdown note covers deploy ordering, which a test cannot enforce.

**Tech Stack:** Go `go/parser`+`go/ast`+`go/token`, `runtime.Caller` for repo-root, `regexp`, `testing`. No new dependencies.

## Global Constraints

- Module path `github.com/wegofwd2020/thittam`. No proto/sqlc/migration/production-code change — only a test file + a doc.
- The test is pure file parsing: NO database, runs under `go test ./... -short`, needs no `ci.yml` edit.
- **The test must PASS against the current tree.** If a first run fails, that is a real finding — fix the seed/migration/allowlist, never loosen an assertion to hide a gap.
- Every touched Go file `gofmt`-clean (`gofmt -l` empty).
- DB safety (binds any subagent): NEVER run `docker compose … -v/down/up` against `infra/local/`. This task needs no DB at all.
- Commits end with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

## Grounding facts (verified on `main` @ f864649)

- `interceptor.RequirePermission(ctx context.Context, checker PermissionChecker, permission string) error` — 3rd arg is the permission (`pkg/interceptor/permission.go:62`).
- `var systemRoles = []struct{ name string; permissions []string }{…}` (unexported, `services/iam/service.go:66`). Field is `permissions` (lowercase). Entries include bare literals AND consts (`permLedgerRead`…`permLedgerAdmin`, `permUserManage`) — reading the var at runtime resolves consts to their string values automatically.
- Ledger gates use package-local consts (`services/ledger/handler.go:26-29`, values `ledger:read/write/post/admin`) at 12 `RequirePermission` sites — the extractor MUST resolve same-package consts.
- Backfill grant shape: `array_append(permissions, 'X')` in `migrations/iam/020..023_*.up.sql`.
- Current sets: **G (24)** = 20 literals + `ledger:{read,write,post,admin}` (+ `user:manage` if iam gates it); **S** = systemRoles perms (⊇ G, also has `inventory:retire`); **B (9)** = `billing:manage, billing:read, document:delete, document:read, document:write, expense:read, inventory:read, notifications:manage, notifications:read`; **F (founding = G\B, 15)** = `budget:approve, budget:read, budget:write, expense:approve, expense:submit, inventory:checkout, inventory:write, production:read, production:write, report:read, resource:manage, ledger:read, ledger:write, ledger:post, ledger:admin` (+ `user:manage` if gated).

---

### Task 1: Coverage test `services/iam/authz_coverage_test.go`

**Files:**
- Create: `services/iam/authz_coverage_test.go` (package `iam`)

**Interfaces:**
- Consumes: the unexported `systemRoles` var (same package). Nothing from other tasks.
- Produces: nothing consumed by later tasks.

**Context:** The test walks the repo from a root located via `runtime.Caller`. Build four string sets — S (from `systemRoles`), G (from `RequirePermission` calls under `services/`, resolving same-package consts), B (from `migrations/iam/*.up.sql`), F (hardcoded founding allowlist) — and assert four inclusions. Skip `_test.go` files when scanning for gates (test literals are not real gates).

- [ ] **Step 1: Write the test skeleton with the four helpers, then the assertions**

Create `services/iam/authz_coverage_test.go`. Structure (fill helper bodies in the next steps; this step establishes the file + `repoRoot` + the assertion shape so it compiles and fails meaningfully):

```go
package iam

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// foundingPermissions are gated before the backfill-migration era: they entered
// systemRoles before any tenant needing a backfill existed, so they have no
// migrations/iam backfill. Adding to this list is a reviewable exception —
// prefer adding a backfill migration for a newly-gated permission (#166).
var foundingPermissions = map[string]bool{
	"budget:approve": true, "budget:read": true, "budget:write": true,
	"expense:approve": true, "expense:submit": true,
	"inventory:checkout": true, "inventory:write": true,
	"production:read": true, "production:write": true,
	"report:read": true, "resource:manage": true,
	"ledger:read": true, "ledger:write": true, "ledger:post": true, "ledger:admin": true,
	// If the first run flags user:manage as gated-non-backfilled, add it here (founding).
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// services/iam/authz_coverage_test.go -> repo root is two dirs up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func TestAuthzCoverage(t *testing.T) {
	root := repoRoot(t)
	seeded := seededPermissions()                 // S
	gated := gatedPermissions(t, filepath.Join(root, "services")) // G
	backfilled := backfilledPermissions(t, filepath.Join(root, "migrations", "iam")) // B

	// 1. G ⊆ S
	if missing := difference(gated, seeded); len(missing) > 0 {
		t.Errorf("gated but not in systemRoles (new tenants can never be granted these): %v", missing)
	}
	// 2. (G \ F) ⊆ B
	nonFounding := map[string]bool{}
	for p := range gated {
		if !foundingPermissions[p] {
			nonFounding[p] = true
		}
	}
	if missing := difference(nonFounding, backfilled); len(missing) > 0 {
		t.Errorf("gated, not founding, and no migrations/iam backfill grants them — existing tenants "+
			"will get PermissionDenied until a backfill migration is added: %v", missing)
	}
	// 3. B ⊆ S
	if missing := difference(backfilled, seeded); len(missing) > 0 {
		t.Errorf("backfilled but not in systemRoles (existing tenants get these, new tenants do not): %v", missing)
	}
	// 4. F ⊆ G  (no stale founding entry)
	gset := gated
	var staleFounding []string
	for p := range foundingPermissions {
		if !gset[p] {
			staleFounding = append(staleFounding, p)
		}
	}
	if len(staleFounding) > 0 {
		sort.Strings(staleFounding)
		t.Errorf("founding allowlist entries no longer gated anywhere — remove them from foundingPermissions: %v", staleFounding)
	}
}

// difference returns sorted keys in a but not in b.
func difference(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 2: Implement `seededPermissions` (set S)**

Reads the in-package `systemRoles` var directly — consts already resolved at runtime:

```go
func seededPermissions() map[string]bool {
	s := map[string]bool{}
	for _, r := range systemRoles {
		for _, p := range r.permissions {
			s[p] = true
		}
	}
	return s
}
```

- [ ] **Step 3: Implement `backfilledPermissions` (set B)**

Regex over `migrations/iam/*.up.sql`:

```go
var arrayAppendRe = regexp.MustCompile(`array_append\(permissions,\s*'([^']+)'\)`)

func backfilledPermissions(t *testing.T, dir string) map[string]bool {
	t.Helper()
	b := map[string]bool{}
	entries, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	for _, f := range entries {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range arrayAppendRe.FindAllStringSubmatch(string(src), -1) {
			b[m[1]] = true
		}
	}
	return b
}
```

- [ ] **Step 4: Implement `gatedPermissions` (set G) with same-package const resolution**

Walk `services/`, parse non-`_test.go` `.go` files grouped by directory, collect per-package string consts, then extract each `RequirePermission` 3rd arg (literal or resolvable const); fail on anything else.

```go
func gatedPermissions(t *testing.T, servicesDir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()

	// Group parsed files by their directory (≈ package).
	filesByDir := map[string][]*ast.File{}
	err := filepath.WalkDir(servicesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		dir := filepath.Dir(path)
		filesByDir[dir] = append(filesByDir[dir], f)
		return nil
	})
	if err != nil {
		t.Fatalf("walk services: %v", err)
	}

	gated := map[string]bool{}
	for _, files := range filesByDir {
		consts := stringConsts(files) // name -> value, this package
		for _, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "RequirePermission" || len(call.Args) < 3 {
					return true
				}
				arg := call.Args[2]
				switch a := arg.(type) {
				case *ast.BasicLit:
					if a.Kind == token.STRING {
						v, _ := strconv.Unquote(a.Value)
						gated[v] = true
						return true
					}
				case *ast.Ident:
					if v, ok := consts[a.Name]; ok {
						gated[v] = true
						return true
					}
				}
				pos := fset.Position(arg.Pos())
				t.Errorf("%s: RequirePermission permission arg is neither a string literal nor a "+
					"resolvable same-package const — the coverage test cannot verify it; use a "+
					"literal/const or extend the resolver", pos)
				return true
			})
		}
	}
	return gated
}

// stringConsts collects `const name = "value"` declarations across the given files.
func stringConsts(files []*ast.File) map[string]string {
	consts := map[string]string{}
	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if v, err := strconv.Unquote(lit.Value); err == nil {
							consts[name.Name] = v
						}
					}
				}
			}
		}
	}
	return consts
}
```

- [ ] **Step 5: Run the test — expect PASS on the current tree**

Run: `go test ./services/iam/ -run TestAuthzCoverage -v`
Expected: PASS. If it FAILS, read which assertion:
- Assertion 1 fail → a gate references a permission not in `systemRoles`: a real bug (add to `systemRoles` + a backfill) or a typo in the gate.
- Assertion 2 fail → a non-founding gated permission has no backfill migration: either add the backfill, or (if it genuinely predates the backfill era) add it to `foundingPermissions`.
- Assertion 3 fail → a backfill grants a permission not in `systemRoles`: add it to `systemRoles`.
- Assertion 4 fail → a `foundingPermissions` entry is no longer gated: remove it.
- "neither a literal nor a resolvable const" → a gate uses a computed/cross-package permission arg: extend the resolver or make it a same-package const.
Resolve the ROOT CAUSE (fix code-under-test or the allowlist); never weaken an assertion.

- [ ] **Step 6: Prove the test has teeth (throwaway edits, NOT committed)**

Temporarily add `if err := interceptor.RequirePermission(ctx, h.perm, "zzz:bogus"); err != nil { return nil, err }` to the top of one expense handler; run `go test ./services/iam/ -run TestAuthzCoverage` → assertion 1 must FAIL naming `zzz:bogus`. Revert.
Temporarily add a gate `"newvertical:read"` (not founding, no backfill) → assertion 2 must FAIL. Revert.
Confirm `git status` is clean of these edits before continuing.

- [ ] **Step 7: gofmt, vet, build, commit**

```bash
gofmt -l services/iam/authz_coverage_test.go   # must print nothing
go vet ./services/iam/
go build ./...
git add services/iam/authz_coverage_test.go
git commit -m "test(iam): coverage test gating permissions must be seeded + backfilled (#166)

A source-parsing test asserting every interceptor.RequirePermission gate
(literal or same-package const, incl. the ledger consts) is present in
systemRoles, that every non-founding gated permission has a migrations/iam
backfill for existing tenants, and that backfills stay consistent with
systemRoles. Rides go test ./... — no CI/k8s/DB change. Prevents shipping a
gate without its grant (the mechanically-preventable core of #166).

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Deploy-ordering note `docs/operations/permission-migration-ordering.md`

**Files:**
- Create: `docs/operations/permission-migration-ordering.md`

**Interfaces:**
- Consumes: nothing. Produces: nothing. (`docs/operations/` already exists from #123.)

**Context:** The test guarantees a backfill migration EXISTS for every newly-gated permission; it cannot guarantee the migration has RUN in prod before the code deploys. That ordering is the operational half.

- [ ] **Step 1: Write the note**

Create `docs/operations/permission-migration-ordering.md`:

```markdown
# Deploying permission-gating changes (#166)

A change that adds or moves an `interceptor.RequirePermission("X")` gate depends on permission `X`
existing on the relevant roles. Two audiences need it:

- **New tenants** get it from `systemRoles` (`services/iam/service.go`) at `CreateTenant`.
- **Existing tenants** get it only when a `migrations/iam` backfill runs
  (`UPDATE roles … array_append(permissions, 'X') …`).

## The rule

**Run `make migrate-all` before rolling the new service code.** Deploying code-first makes every
existing tenant return `PermissionDenied` on the newly-gated RPCs until the backfill migration runs —
migration-first is harmless (the permission simply exists before anything checks it).

## What the automated checks do and do not cover

- `services/iam/authz_coverage_test.go` (rides `go test ./...`) guarantees that for every gated
  permission a grant EXISTS — it is in `systemRoles`, and every non-founding one has a backfill
  migration. It does NOT guarantee the migration has RUN in a given environment. Ordering is still the
  operator's responsibility.
- CI `Migration Validate (up + down)` runs against a fresh EMPTY database, so a data migration like
  `020_seed_read_permissions` executes against zero rows. It validates SQL syntax and that `down` does
  not error — NOT that the grant reaches any real tenant's roles. Semantic coverage lives in the slice
  integration tests, not that job.
```

- [ ] **Step 2: Sanity-check links + commit**

```bash
test -f services/iam/authz_coverage_test.go && echo "test referenced by note exists"
grep -n "migrate-all\|systemRoles\|Migration Validate" docs/operations/permission-migration-ordering.md
git add docs/operations/permission-migration-ordering.md
git commit -m "docs(ops): note permission-migration deploy ordering (#166)

migrate-all before rolling permission-gating code; the coverage test proves
the backfill EXISTS, not that it has RUN; Migration Validate is syntax-only.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

- **Spec coverage:** Artifact 1 (coverage test, 3 invariants + allowlist-honesty) → Task 1; Artifact 2 (ordering note) → Task 2. Spec's const-resolution requirement → Task 1 Step 4 (`stringConsts` + Ident resolution). Spec's teeth-demonstration → Task 1 Step 6. Spec's "no ci.yml change / rides go test" → the test is an ordinary `_test.go`. All covered.
- **Placeholder scan:** all code blocks are complete and compilable; the `foundingPermissions` map is the concrete 15-entry set with a documented add-more rule; no TBD/TODO. The `user:manage` note in Step 5/allowlist is a real conditional instruction, not a placeholder.
- **Type consistency:** `systemRoles` field `permissions` (lowercase) used in `seededPermissions`. `map[string]bool` sets used consistently across `seededPermissions`/`gatedPermissions`/`backfilledPermissions`/`difference`. `RequirePermission` selector name + 3rd-arg (index 2) match `pkg/interceptor/permission.go`. Test name `TestAuthzCoverage` matches the gate commands in Steps 5-7.
