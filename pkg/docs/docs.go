// Package docs centralises URLs to Thittam's external documentation
// (thittam_docs repo). Use these constants when Go code needs to emit
// a link to a documentation page at runtime — in an error message, a
// log entry, an API response field, etc. A doc reorg then updates one
// file in this package instead of a find-replace across the codebase.
//
// Limitation: Go doc comments cannot interpolate constants, so links
// inside `// see docs/X` comments remain plain strings and must be
// hand-updated when docs move. That's a fundamental Go limitation, not
// something this package can solve.
package docs

// Repo is the thittam_docs GitHub repository URL.
const Repo = "https://github.com/wegofwd2020-hub/thittam_docs"

// Base is the browse-main URL prefix. Append a repo-relative doc path
// (e.g., "/docs/developers/adr/ADR-015-...") to build a full link.
const Base = Repo + "/blob/main"

// Well-known documents referenced from Go code. Add entries here when
// a doc becomes load-bearing in a runtime code path (not just in a
// comment).
const (
	TenantOnboardingGuide = Base + "/docs/developers/operations/tenant-onboarding.md"
	ADR015PaymentGateway  = Base + "/docs/developers/adr/ADR-015-payment-gateway-abstraction.md"
	BillingPlanLimits     = Base + "/docs/developers/services/billing.md"
	APIErrorHandling      = Base + "/docs/developers/api/error-handling.md"
)
