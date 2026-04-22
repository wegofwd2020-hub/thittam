// gen-api-docs walks proto/**/*.proto and emits two markdown files:
//
//	public-api.md   — RPCs with option (google.api.http)  — REST-exposed via Kong
//	internal-api.md — RPCs without that option           — service-to-service gRPC only
//
// Usage (from thittam repo root):
//
//	go run ./tools/gen-api-docs -proto proto -out ../thittam_docs/docs/developers/api/generated
//
// Flags:
//
//	-proto      Proto root directory (default: proto)
//	-out        Output directory (must exist)
//	-source-ref Source commit SHA or ref for the generated-header line
//	-source-url Base URL for linking back to proto files (default: the public repo)
//
// No external Go dependencies — parses proto text with regex. Sufficient
// because Thittam's proto files are well-formed; a full AST parser can
// replace this later if needed.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type rpc struct {
	name        string
	request     string
	response    string
	description string
	httpMethod  string // "" if internal
	httpPath    string // "" if internal
}

type service struct {
	name string
	file string // relative path from proto root, e.g. thittam/project/v1/project.proto
	rpcs []rpc
}

// servicePorts maps service names → port numbers for the public-api header.
// Kept in code (not parsed) because ports live in deployment config, not proto.
// Update when a new service lands.
var servicePorts = map[string]int{
	"ProjectService":       8080,
	"BudgetService":        8081,
	"ExpenseService":       8082,
	"GeneralLedgerService": 8083,
	"InventoryService":     8084,
	"ReportingService":     8085,
	"IAMService":           8086,
	"NotificationService":  8087,
	"DocumentService":      8088,
	"BillingService":       8089,
}

var (
	serviceRe = regexp.MustCompile(`(?s)service\s+(\w+)\s*\{(.*?)\n\}`)
	// RPC signature up to (but not including) the trailing `;` or `{…}`.
	// The body is extracted separately because it can contain nested braces.
	rpcHeadRe = regexp.MustCompile(`rpc\s+(\w+)\s*\(\s*([\w.]+)\s*\)\s+returns\s+\(\s*([\w.]+)\s*\)\s*`)
	// The http option inside an RPC body.
	httpRe = regexp.MustCompile(`option\s+\(google\.api\.http\)\s*=\s*\{\s*(\w+):\s*"([^"]+)"`)
)

// extractBody starts at pos in text. If text[pos] is ';' it returns ("", next).
// If text[pos] is '{' it returns the body between the outer braces (exclusive)
// using brace-depth counting, plus the index just past the closing '}'.
func extractBody(text string, pos int) (body string, next int, ok bool) {
	if pos >= len(text) {
		return "", pos, false
	}
	switch text[pos] {
	case ';':
		return "", pos + 1, true
	case '{':
		depth := 1
		for i := pos + 1; i < len(text); i++ {
			switch text[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return text[pos+1 : i], i + 1, true
				}
			}
		}
	}
	return "", pos, false
}

func parseFile(path, relPath string) ([]service, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(b)
	lines := strings.Split(text, "\n")

	var out []service
	for _, svcMatch := range serviceRe.FindAllStringSubmatchIndex(text, -1) {
		svcName := text[svcMatch[2]:svcMatch[3]]
		body := text[svcMatch[4]:svcMatch[5]]

		svc := service{name: svcName, file: relPath}
		// Walk the service body, pulling RPCs one at a time so we can use
		// brace-depth counting for each body (which may contain nested {…}).
		offset := 0
		for offset < len(body) {
			m := rpcHeadRe.FindStringSubmatchIndex(body[offset:])
			if m == nil {
				break
			}
			name := body[offset+m[2] : offset+m[3]]
			req := body[offset+m[4] : offset+m[5]]
			resp := body[offset+m[6] : offset+m[7]]
			bodyStart := offset + m[1]
			rpcBody, next, ok := extractBody(body, bodyStart)
			if !ok {
				offset = bodyStart + 1
				continue
			}
			r := rpc{name: name, request: req, response: resp}
			if hm := httpRe.FindStringSubmatch(rpcBody); hm != nil {
				r.httpMethod = strings.ToUpper(hm[1])
				r.httpPath = hm[2]
			}
			r.description = precedingComment(lines, "", name)
			svc.rpcs = append(svc.rpcs, r)
			offset = next
		}
		if len(svc.rpcs) > 0 {
			out = append(out, svc)
		}
	}
	return out, nil
}

// precedingComment gathers consecutive `//` lines immediately preceding the
// RPC (no blank line between), joins them, and returns the first sentence.
// A blank line or any non-comment line terminates the block.
// Returns "" if no preceding comment block exists.
func precedingComment(fileLines []string, _ string, rpcName string) string {
	target := "rpc " + rpcName
	for i, ln := range fileLines {
		if !strings.Contains(ln, target) {
			continue
		}
		var comments []string
		for j := i - 1; j >= 0; j-- {
			s := strings.TrimSpace(fileLines[j])
			if strings.HasPrefix(s, "//") {
				comments = append([]string{strings.TrimSpace(strings.TrimPrefix(s, "//"))}, comments...)
				continue
			}
			break
		}
		if len(comments) == 0 {
			return ""
		}
		joined := strings.Join(comments, " ")
		// Drop section-marker comments like `--- Subscriptions ---`: they
		// describe a group of RPCs, not this specific RPC.
		if strings.HasPrefix(joined, "---") {
			return ""
		}
		// First sentence, if any.
		if dot := strings.Index(joined, ". "); dot > 0 {
			return joined[:dot+1]
		}
		return joined
	}
	return ""
}

func main() {
	var (
		protoRoot = flag.String("proto", "proto", "Proto root directory")
		outDir    = flag.String("out", ".", "Output directory (must exist)")
		sourceRef = flag.String("source-ref", "", "Source commit SHA or ref for header")
		sourceURL = flag.String("source-url", "https://github.com/wegofwd2020-hub/thittam", "Base URL for linking proto files")
	)
	flag.Parse()

	if _, err := os.Stat(*outDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: out dir %q: %v\n", *outDir, err)
		os.Exit(2)
	}

	var services []service
	err := filepath.WalkDir(*protoRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".proto") {
			return err
		}
		rel, _ := filepath.Rel(*protoRoot, p)
		svcs, err := parseFile(p, rel)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		services = append(services, svcs...)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}

	sort.Slice(services, func(i, j int) bool { return services[i].name < services[j].name })

	date := time.Now().UTC().Format("2006-01-02")
	refLine := ""
	if *sourceRef != "" {
		refLine = fmt.Sprintf(" from source ref `%s`", *sourceRef)
	}
	header := func(title, subtitle string) string {
		return fmt.Sprintf(`<!-- GENERATED by tools/gen-api-docs — DO NOT EDIT DIRECTLY -->
<!-- Regenerated %s%s. Source of truth: proto files in wegofwd2020-hub/thittam. -->

# %s

%s

`, date, refLine, title, subtitle)
	}

	publicBody := emitPublic(services, *sourceURL)
	internalBody := emitInternal(services, *sourceURL)

	publicPath := filepath.Join(*outDir, "public-api.md")
	internalPath := filepath.Join(*outDir, "internal-api.md")

	publicDoc := header(
		"Thittam Public API — REST Endpoints",
		`REST endpoints exposed via Kong API Gateway, transcoded from gRPC.
These are the external contracts available to customers, integrators,
and partners.

Internal-only gRPC methods (not exposed as REST) are in
[`+"`internal-api.md`"+`](internal-api.md). The raw proto message
reference is in [`+"`PROTO_REFERENCE.md`"+`](PROTO_REFERENCE.md).`) + publicBody

	internalDoc := header(
		"Thittam Internal API — gRPC Methods",
		`gRPC methods for service-to-service communication. Not exposed
through Kong; not part of the external contract. Internal callers
only (other Thittam services, trusted sidecars).

Publicly exposed REST endpoints are in [`+"`public-api.md`"+`](public-api.md).
The raw proto message reference is in [`+"`PROTO_REFERENCE.md`"+`](PROTO_REFERENCE.md).`) + internalBody

	if err := os.WriteFile(publicPath, []byte(publicDoc), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write public:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(internalPath, []byte(internalDoc), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write internal:", err)
		os.Exit(1)
	}

	// Summary to stdout so the CI log shows the diff shape.
	var pubCount, intCount int
	for _, s := range services {
		for _, r := range s.rpcs {
			if r.httpMethod != "" {
				pubCount++
			} else {
				intCount++
			}
		}
	}
	fmt.Printf("wrote %s (%d public RPCs)\n", publicPath, pubCount)
	fmt.Printf("wrote %s (%d internal RPCs)\n", internalPath, intCount)
}

func emitPublic(services []service, sourceURL string) string {
	var sb strings.Builder
	any := false
	for _, s := range services {
		var rows []rpc
		for _, r := range s.rpcs {
			if r.httpMethod != "" {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 {
			continue
		}
		any = true
		port := ""
		if p, ok := servicePorts[s.name]; ok {
			port = fmt.Sprintf(" (port %d)", p)
		}
		fmt.Fprintf(&sb, "## %s%s\n\n", s.name, port)
		fmt.Fprintf(&sb, "Source: [`%s`](%s/blob/main/proto/%s)\n\n", s.file, sourceURL, s.file)
		fmt.Fprintln(&sb, "| Method | Path | gRPC method | Description |")
		fmt.Fprintln(&sb, "|---|---|---|---|")
		for _, r := range rows {
			desc := r.description
			if desc == "" {
				desc = "—"
			}
			fmt.Fprintf(&sb, "| `%s` | `%s` | `%s` | %s |\n", r.httpMethod, r.httpPath, r.name, desc)
		}
		fmt.Fprintln(&sb)
	}
	if !any {
		sb.WriteString("_No public REST endpoints defined yet._\n")
	}
	return sb.String()
}

func emitInternal(services []service, sourceURL string) string {
	var sb strings.Builder
	any := false
	for _, s := range services {
		var rows []rpc
		for _, r := range s.rpcs {
			if r.httpMethod == "" {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 {
			continue
		}
		any = true
		port := ""
		if p, ok := servicePorts[s.name]; ok {
			port = fmt.Sprintf(" (port %d)", p)
		}
		fmt.Fprintf(&sb, "## %s%s\n\n", s.name, port)
		fmt.Fprintf(&sb, "Source: [`%s`](%s/blob/main/proto/%s)\n\n", s.file, sourceURL, s.file)
		fmt.Fprintln(&sb, "| RPC | Request | Response | Description |")
		fmt.Fprintln(&sb, "|---|---|---|---|")
		for _, r := range rows {
			desc := r.description
			if desc == "" {
				desc = "—"
			}
			fmt.Fprintf(&sb, "| `%s` | `%s` | `%s` | %s |\n", r.name, r.request, r.response, desc)
		}
		fmt.Fprintln(&sb)
	}
	if !any {
		sb.WriteString("_No internal-only gRPC methods._\n")
	}
	return sb.String()
}
