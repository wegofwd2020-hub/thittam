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
	// Note: iam gates user:manage via requireUserManage (direct svc.CheckPermission), not
	// interceptor.RequirePermission, so it is not in the gated set G and needs no entry here.
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
	seeded := seededPermissions()                                                    // S
	gated := gatedPermissions(t, filepath.Join(root, "services"))                    // G
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

func seededPermissions() map[string]bool {
	s := map[string]bool{}
	for _, r := range systemRoles {
		for _, p := range r.permissions {
			s[p] = true
		}
	}
	return s
}

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

// gatedPermissions returns the set of permission strings gated via
// interceptor.RequirePermission(ctx, checker, X) calls (X a string literal or a
// resolvable same-package const) across the given services tree. By design this
// does NOT capture gates done another way — e.g. iam's requireUserManage, which
// calls svc.CheckPermission directly instead of going through the interceptor
// because iam cannot dial itself for a PermissionChecker. Such gates are out of
// scope for this extractor and this test.
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
