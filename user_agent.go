package humantone

import (
	"runtime"
	"strings"
)

// buildUserAgent assembles the User-Agent header value from the SDK version,
// the runtime Go version (with the "go" prefix stripped), and an optional
// user-supplied suffix. Suffix whitespace is trimmed; an empty trimmed suffix
// is omitted entirely (no trailing space).
func buildUserAgent(sdkVersion, goVersion, suffix string) string {
	base := "humantone-go/" + sdkVersion + " (go/" + goVersion + ")"
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return base
	}
	return base + " " + suffix
}

// sanitizedGoVersion strips the leading "go" prefix from runtime.Version().
// On the standard Go toolchain this produces "1.22.3" from "go1.22.3". For
// non-standard toolchains (e.g., gccgo) the result is whatever remains after
// trimming "go", which is acceptable for User-Agent purposes.
func sanitizedGoVersion() string {
	return strings.TrimPrefix(runtime.Version(), "go")
}
