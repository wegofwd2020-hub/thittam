package registration

import (
	"regexp"
	"strings"
)

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9\-]+`)
	multiDash       = regexp.MustCompile(`-{2,}`)
)

// Slugify converts a company name to a URL-safe slug.
// "Acme Software Pvt. Ltd." → "acme-software-pvt-ltd"
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumeric.ReplaceAllString(s, "")
	s = multiDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
