// check-doc-drift scans Markdown documentation for Go identifier references
// inside ```go ... ``` code fences and verifies that each referenced identifier
// still exists somewhere in the application Go source tree.
//
// # How it works
//
//  1. Walk every *.md file under --docs.
//  2. For each file, extract ```go ... ``` blocks.
//  3. From each block, extract exported Go identifiers (func, type, var/const
//     declarations). A line like "func NewFoo(" yields identifier "NewFoo".
//  4. Build a word-level index of every token that starts with an uppercase
//     letter appearing in any *.go file under --app.
//  5. For each doc identifier not present in the index, emit a FAIL line.
//  6. Exit 1 if any FAILs were found, 0 otherwise.
//
// # Escape hatch
//
// Place a <!-- drift:ignore --> HTML comment on the line immediately before
// a ```go block to skip extraction for that block. Use this for pseudocode or
// intentionally simplified snippets that do not reflect real API names.
//
// # Usage
//
//	go run ./tools/check-doc-drift \
//	    --docs  ../thittam_docs \
//	    --app   .
//	    [--verbose]
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// identPatterns extracts the first exported identifier from a line in a Go
// code block. Order matters — method pattern before function pattern so that
// the method name (not the receiver) is captured when both could match.
var identPatterns = []*regexp.Regexp{
	// Method declaration:   func (r *ReceiverType) MethodName(
	regexp.MustCompile(`^\s*func\s*\([^)]+\)\s+([A-Z][a-zA-Z0-9]+)\s*[(\[]`),
	// Top-level function:   func FunctionName(
	regexp.MustCompile(`^\s*func\s+([A-Z][a-zA-Z0-9]+)\s*[(\[]`),
	// Type declaration:     type TypeName struct / interface / =
	regexp.MustCompile(`^\s*type\s+([A-Z][a-zA-Z0-9]+)\s+`),
	// Var / const:          var ErrFoo  or  const MaxFoo
	regexp.MustCompile(`^\s*(?:var|const)\s+([A-Z][a-zA-Z0-9]+)\b`),
}

// wordBoundary matches word-boundary separators so we can do whole-word checks
// without importing regexp per identifier.
var nonWordChar = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// --------------------------------------------------------------------------
// Types
// --------------------------------------------------------------------------

// finding records an exported identifier extracted from a doc file together
// with the location it was found.
type finding struct {
	docFile string
	line    int
	ident   string
}

// --------------------------------------------------------------------------
// Doc scanning
// --------------------------------------------------------------------------

// extractFindings walks docsDir for *.md files and returns all exported
// identifiers found inside ```go ... ``` blocks (excluding drift:ignore ones).
func extractFindings(docsDir string) ([]finding, error) {
	var findings []finding

	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		ff, ferr := scanMarkdownFile(path)
		if ferr != nil {
			return fmt.Errorf("scan %s: %w", path, ferr)
		}
		findings = append(findings, ff...)
		return nil
	})
	return findings, err
}

// scanMarkdownFile parses one Markdown file and returns findings.
func scanMarkdownFile(path string) ([]finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		findings      []finding
		inGoBlock     bool
		ignoreBlock   bool // true when the current block was preceded by <!-- drift:ignore -->
		pendingIgnore bool // set by <!-- drift:ignore -->, consumed when next fence opens
		blockBuf      []string
		blockStart    int
		lineNum       int
	)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inGoBlock {
			// Check for the ignore marker.
			if strings.Contains(trimmed, "<!-- drift:ignore -->") {
				pendingIgnore = true
				continue
			}
			// Detect opening fence: ```go (optionally followed by whitespace or end-of-line).
			if trimmed == "```go" || strings.HasPrefix(trimmed, "```go ") || strings.HasPrefix(trimmed, "```go\t") {
				inGoBlock = true
				ignoreBlock = pendingIgnore
				pendingIgnore = false
				blockBuf = nil
				blockStart = lineNum
				continue
			}
			// Any non-fence, non-ignore line resets the pending-ignore flag.
			// This prevents an ignore marker from skipping a block that isn't immediately next.
			if trimmed != "" {
				pendingIgnore = false
			}
			continue
		}

		// Inside a code block.
		if trimmed == "```" || strings.HasPrefix(trimmed, "``` ") {
			// Closing fence — extract identifiers unless this block was ignored.
			if !ignoreBlock {
				for i, blockLine := range blockBuf {
					if ident := extractIdent(blockLine); ident != "" {
						findings = append(findings, finding{
							docFile: path,
							line:    blockStart + 1 + i,
							ident:   ident,
						})
					}
				}
			}
			inGoBlock = false
			ignoreBlock = false
			blockBuf = nil
			continue
		}
		blockBuf = append(blockBuf, line)
	}
	return findings, scanner.Err()
}

// extractIdent returns the first exported identifier found on a line of Go
// source (inside a code block), or "" if none is found.
// Identifiers shorter than 3 characters are skipped — they are too generic
// to be meaningful (e.g. "DB", "Is") and cause false positives.
func extractIdent(line string) string {
	for _, re := range identPatterns {
		if m := re.FindStringSubmatch(line); m != nil {
			if len(m[1]) >= 3 {
				return m[1]
			}
		}
	}
	return ""
}

// --------------------------------------------------------------------------
// App source indexing
// --------------------------------------------------------------------------

// buildIndex returns the set of all exported-looking tokens (starts with
// an uppercase ASCII letter, length ≥ 3) appearing in any *.go file under
// appDir. Using a word-level search (not just declarations) means docs that
// reference an identifier by its full name will match even when the identifier
// appears in comments, generated code, or type embeddings.
func buildIndex(appDir string) (map[string]bool, error) {
	index := make(map[string]bool, 4096)

	err := filepath.Walk(appDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip vendor, .git, node_modules.
			base := info.Name()
			if base == "vendor" || base == ".git" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		return indexFile(path, index)
	})
	return index, err
}

// indexFile adds all exported tokens from a Go source file to index.
func indexFile(path string, index map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Split the line on any non-word character and collect exported tokens.
		tokens := nonWordChar.Split(line, -1)
		for _, tok := range tokens {
			if len(tok) < 3 {
				continue
			}
			r := rune(tok[0])
			if unicode.IsUpper(r) && unicode.IsLetter(r) {
				index[tok] = true
			}
		}
	}
	return scanner.Err()
}

// --------------------------------------------------------------------------
// Reporting
// --------------------------------------------------------------------------

// miss holds a finding that was not found in the app index.
type miss struct {
	finding
}

func run() int {
	docsDir := flag.String("docs", "", "Path to the documentation repository root (required)")
	appDir := flag.String("app", ".", "Path to the application repository root")
	verbose := flag.Bool("verbose", false, "Print all checked identifiers, not just failures")
	flag.Parse()

	if *docsDir == "" {
		fmt.Fprintln(os.Stderr, "check-doc-drift: --docs is required")
		flag.Usage()
		return 2
	}

	// Resolve paths.
	docsAbs, err := filepath.Abs(*docsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-doc-drift: resolve docs path: %v\n", err)
		return 2
	}
	appAbs, err := filepath.Abs(*appDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-doc-drift: resolve app path: %v\n", err)
		return 2
	}

	// Extract identifiers from docs.
	findings, err := extractFindings(docsAbs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-doc-drift: scan docs: %v\n", err)
		return 2
	}

	// Deduplicate across findings (same identifier may appear in multiple docs).
	seen := make(map[string]bool, len(findings))
	var unique []finding
	for _, f := range findings {
		if !seen[f.ident] {
			seen[f.ident] = true
			unique = append(unique, f)
		}
	}

	if len(unique) == 0 {
		fmt.Println("check-doc-drift: no Go identifiers found in docs — nothing to check")
		return 0
	}

	// Build app source index.
	index, err := buildIndex(appAbs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-doc-drift: index app source: %v\n", err)
		return 2
	}

	// Check each unique identifier.
	var misses []miss
	var checked []finding
	for _, f := range unique {
		if index[f.ident] {
			checked = append(checked, f)
		} else {
			misses = append(misses, miss{f})
		}
	}

	// Sort misses by file then line for deterministic output.
	sort.Slice(misses, func(i, j int) bool {
		if misses[i].docFile != misses[j].docFile {
			return misses[i].docFile < misses[j].docFile
		}
		return misses[i].line < misses[j].line
	})

	if *verbose {
		fmt.Printf("check-doc-drift: indexed %d exported tokens from %s\n", len(index), appAbs)
		fmt.Printf("check-doc-drift: checked %d unique identifier(s)\n", len(unique))
		for _, f := range checked {
			// Trim the docs root prefix for readability.
			rel, _ := filepath.Rel(docsAbs, f.docFile)
			fmt.Printf("  OK  %-40s  %s:%d\n", f.ident, rel, f.line)
		}
	}

	if len(misses) == 0 {
		fmt.Printf("check-doc-drift: all %d documented identifier(s) found in codebase\n", len(checked))
		return 0
	}

	fmt.Printf("check-doc-drift: FAIL — %d identifier(s) documented but not found in %s\n\n",
		len(misses), appAbs)
	for _, m := range misses {
		rel, _ := filepath.Rel(docsAbs, m.docFile)
		fmt.Printf("  MISS  %-40s  %s:%d\n", m.ident, rel, m.line)
	}
	fmt.Printf("\nFix: update the documentation to match the current code, or add\n")
	fmt.Printf("<!-- drift:ignore --> before the code block if the snippet is intentional pseudocode.\n")
	return 1
}

func main() {
	os.Exit(run())
}
