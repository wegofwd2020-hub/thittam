# gen-api-docs

Walks `proto/**/*.proto` and emits two markdown files that split the
gRPC surface into what's public (REST-exposed via Kong) and what's
internal (service-to-service gRPC only):

- `public-api.md`   — RPCs with `option (google.api.http) = {...}`
- `internal-api.md` — RPCs without that option

Run nightly from `thittam_docs/.github/workflows/api-docs-sync.yml`;
that workflow opens a PR against `thittam_docs` when anything drifts.

## Run locally

```bash
# From thittam repo root, writing into the sibling thittam_docs checkout:
go run ./tools/gen-api-docs \
  -proto proto \
  -out ../thittam_docs/docs/developers/api/generated \
  -source-ref "$(git rev-parse --short HEAD)"
```

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `-proto` | `proto` | Proto root directory |
| `-out` | `.` | Output directory (must exist) |
| `-source-ref` | "" | Commit SHA / ref for the "generated from" header |
| `-source-url` | `https://github.com/wegofwd2020-hub/thittam` | Base URL for linking proto files |

## Public-vs-internal marker

The split is a byproduct of `.proto` structure — no custom
annotation to maintain:

```protobuf
// Public (REST-exposed)
rpc GetProduction(GetProductionRequest) returns (Production) {
  option (google.api.http) = { get: "/api/v1/productions/{id}" };
}

// Internal (gRPC-only)
rpc ArchiveProduction(ArchiveProductionRequest) returns (Production);
```

Move an RPC between public and internal by adding / removing the
`google.api.http` option — the nightly sync PR picks it up.

## Implementation notes

No external Go dependencies. Proto text is parsed with a
brace-depth scanner plus small regexes; sufficient because the
repo's proto files are well-formed. A full AST parser (`buf`,
`emicklei/proto`) can replace this later if the grammar gets
ambitious (streaming, options-inside-options, etc.).

Descriptions come from `//` lines immediately preceding the RPC
(no blank line between). Block-level section markers like
`// --- Section ---` are filtered out — they describe groups, not
specific RPCs.
