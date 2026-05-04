package humantone

import (
	"regexp"
	"testing"
)

func TestBuildUserAgentDefault(t *testing.T) {
	got := buildUserAgent("0.0.1", "1.22.3", "")
	want := "humantone-go/0.0.1 (go/1.22.3)"
	if got != want {
		t.Errorf("buildUserAgent default = %q, want %q", got, want)
	}
}

func TestBuildUserAgentWithSuffix(t *testing.T) {
	got := buildUserAgent("0.0.1", "1.22.3", "my-app/1.0")
	want := "humantone-go/0.0.1 (go/1.22.3) my-app/1.0"
	if got != want {
		t.Errorf("buildUserAgent with suffix = %q, want %q", got, want)
	}
}

func TestBuildUserAgentEmptyAndWhitespaceSuffix(t *testing.T) {
	cases := []string{"", "   ", "\t", "\n  \t"}
	want := "humantone-go/0.0.1 (go/1.22.3)"
	for _, suffix := range cases {
		got := buildUserAgent("0.0.1", "1.22.3", suffix)
		if got != want {
			t.Errorf("buildUserAgent(suffix=%q) = %q, want %q (no trailing space)", suffix, got, want)
		}
	}
}

func TestBuildUserAgentTrimsPaddedSuffix(t *testing.T) {
	got := buildUserAgent("0.0.1", "1.22.3", "  my-app/1.0  ")
	want := "humantone-go/0.0.1 (go/1.22.3) my-app/1.0"
	if got != want {
		t.Errorf("buildUserAgent trims = %q, want %q", got, want)
	}
}

// TestSanitizedGoVersionShape asserts the structural shape of the live-runtime
// User-Agent without depending on the specific Go version installed on the
// test runner. Per §9.1.
func TestSanitizedGoVersionShape(t *testing.T) {
	ua := buildUserAgent(SDKVersion, sanitizedGoVersion(), "")
	pattern := regexp.MustCompile(`^humantone-go/\d+\.\d+\.\d+(?:-[a-zA-Z0-9.]+)? \(go/[A-Za-z0-9.+_-]+\)$`)
	if !pattern.MatchString(ua) {
		t.Errorf("sanitized UA %q does not match expected shape %s", ua, pattern)
	}
}
