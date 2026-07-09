package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"short string unchanged", "hello", 60, "hello"},
		{"exactly n unchanged", "abcdef", 6, "abcdef"},
		{"long ascii cut with ellipsis", strings.Repeat("a", 70), 60, strings.Repeat("a", 59) + "…"},
		{"n<=0 empty, no panic", "non-empty", 0, ""},
		{"negative n empty, no panic", "non-empty", -5, ""},
		{"empty string, n<=0", "", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.in, tc.n)
			if got != tc.want {
				t.Fatalf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("truncate(%q, %d) produced invalid UTF-8: %q", tc.in, tc.n, got)
			}
		})
	}
}

func TestTruncateMultiByteUTF8Boundary(t *testing.T) {
	// 30 "é" (2 bytes each in UTF-8) plus an ASCII tail, cut at n=60. A
	// byte-index slice at s[:59] would land mid-rune and emit invalid UTF-8;
	// a rune-aware cut must not.
	s := strings.Repeat("é", 30) + "tail of ascii text to push well past the limit"

	got := truncate(s, 60)

	if !utf8.ValidString(got) {
		t.Fatalf("truncate produced invalid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != 60 {
		t.Fatalf("truncate(%q, 60) rune count = %d, want 60", s, n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncate(%q, 60) = %q, want ellipsis suffix", s, got)
	}
}

func TestTruncateNoPanicOnNonPositiveN(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("truncate panicked: %v", r)
		}
	}()
	for _, n := range []int{0, -1, -100} {
		_ = truncate("some non-empty string", n)
	}
}

func TestDeadCountNotice(t *testing.T) {
	statsErr := errors.New("connection reset by peer")

	cases := []struct {
		name       string
		shown      int
		total      int64
		statsErr   error
		wantOK     bool
		wantRemain int64
		wantWarn   bool
	}{
		{
			name:     "no error, dead equals shown: no notice",
			shown:    7,
			total:    7,
			statsErr: nil,
			wantOK:   false,
		},
		{
			name:     "no error, dead less than shown (stale/inconsistent snapshot): no notice",
			shown:    7,
			total:    5,
			statsErr: nil,
			wantOK:   false,
		},
		{
			name:       "no error, dead exceeds shown: notice names exact remaining count",
			shown:      100,
			total:      137,
			statsErr:   nil,
			wantOK:     true,
			wantRemain: 37,
		},
		{
			name:     "stats error: warning, no notice, no drained implication",
			shown:    100,
			total:    0,
			statsErr: statsErr,
			wantOK:   false,
			wantWarn: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remaining, ok, warn := deadCountNotice(tc.shown, tc.total, tc.statsErr)

			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && remaining != tc.wantRemain {
				t.Fatalf("remaining = %d, want %d", remaining, tc.wantRemain)
			}
			if tc.wantWarn && warn == "" {
				t.Fatal("expected a non-empty warning, got \"\"")
			}
			if !tc.wantWarn && warn != "" {
				t.Fatalf("expected no warning, got: %q", warn)
			}
			if tc.statsErr != nil && !strings.Contains(warn, tc.statsErr.Error()) {
				t.Fatalf("warning %q does not mention underlying error %q", warn, tc.statsErr.Error())
			}
			// A stats failure must never come back looking like "queue
			// drained" (ok=false with an empty warn is indistinguishable
			// from the legitimate zero-remaining case) — it must always
			// carry an explicit warning explaining the count is unknown.
			if tc.statsErr != nil && (ok || warn == "") {
				t.Fatalf("stats error must produce ok=false with a non-empty warning, got ok=%v warn=%q", ok, warn)
			}
		})
	}
}

// withoutDatabaseURL unsets DATABASE_URL for the duration of the test and
// restores whatever was there before, so these tests never depend on (or
// pollute) the ambient environment.
func withoutDatabaseURL(t *testing.T) {
	t.Helper()
	prev, had := os.LookupEnv("DATABASE_URL")
	if err := os.Unsetenv("DATABASE_URL"); err != nil {
		t.Fatalf("unsetenv DATABASE_URL: %v", err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("DATABASE_URL", prev)
		}
	})
}

func TestReplayMutualExclusion_NeitherFlagSet(t *testing.T) {
	withoutDatabaseURL(t)

	cmd := newReplayCmd()
	cmd.SetArgs([]string{})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when neither --id nor --all is set, got nil")
	}
	if !strings.Contains(err.Error(), "specify exactly one of --id or --all") {
		t.Fatalf("expected mutual-exclusion error, got: %v", err)
	}
}

func TestReplayMutualExclusion_BothFlagsSet(t *testing.T) {
	withoutDatabaseURL(t)

	cmd := newReplayCmd()
	cmd.SetArgs([]string{"--id", "abc", "--all"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --id and --all are set, got nil")
	}
	if !strings.Contains(err.Error(), "specify exactly one of --id or --all") {
		t.Fatalf("expected mutual-exclusion error, got: %v", err)
	}
	// This must be rejected before any DB connection is attempted: the
	// error must be the validation error, not a missing-DATABASE_URL error.
	if errors.Is(err, errMissingDSN) {
		t.Fatalf("expected validation error, got missing-DSN error: %v", err)
	}
}

func TestReplayExactlyAll_PassesValidationBeforeDBConnect(t *testing.T) {
	withoutDatabaseURL(t)

	cmd := newReplayCmd()
	cmd.SetArgs([]string{"--all"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error (missing DATABASE_URL), got nil")
	}
	// Getting errMissingDSN (rather than the mutual-exclusion error) proves
	// --all alone passed validation and reached openRepo, whose only
	// failure mode here (no DATABASE_URL set, no network access) is the
	// missing-DSN sentinel — i.e. no real DB connection was attempted.
	if !errors.Is(err, errMissingDSN) {
		t.Fatalf("expected errMissingDSN, got: %v", err)
	}
}

func TestReplayExactlyID_ValidUUID_PassesValidationBeforeDBConnect(t *testing.T) {
	withoutDatabaseURL(t)

	cmd := newReplayCmd()
	cmd.SetArgs([]string{"--id", "8c9a8c2e-6f3b-4a2b-9c3d-1e2f3a4b5c6d"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error (missing DATABASE_URL), got nil")
	}
	if !errors.Is(err, errMissingDSN) {
		t.Fatalf("expected errMissingDSN (uuid should have parsed fine), got: %v", err)
	}
}

func TestReplayID_InvalidUUID_FailsBeforeDBConnect(t *testing.T) {
	withoutDatabaseURL(t)

	cmd := newReplayCmd()
	cmd.SetArgs([]string{"--id", "notauuid"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an invalid-UUID error, got nil")
	}
	if errors.Is(err, errMissingDSN) {
		t.Fatalf("expected invalid-UUID error, got missing-DSN error (means DB connect was attempted first): %v", err)
	}
	if !strings.Contains(err.Error(), "invalid --id") {
		t.Fatalf("expected invalid --id error, got: %v", err)
	}
}
