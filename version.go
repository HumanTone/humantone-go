package humantone

import (
	"runtime/debug"
	"strings"
)

// SDKVersion is the hardcoded version of this SDK. It is bumped manually before
// each git tag and serves as a fallback when runtime build-info lookup does not
// surface a usable module version (for example when the SDK is compiled as the
// main module during development).
const SDKVersion = "0.0.1"

// modulePath is the canonical Go module path for this SDK. It is used to find
// the SDK's own dependency entry in runtime build info.
const modulePath = "github.com/humantone/humantone-go"

// resolveSDKVersion returns the SDK version reported in the consumer's build
// info, falling back to the SDKVersion constant when build info is unavailable
// or reports an unhelpful value such as "(devel)".
func resolveSDKVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return SDKVersion
	}
	for _, dep := range info.Deps {
		if dep.Path != modulePath {
			continue
		}
		if dep.Version == "" || dep.Version == "(devel)" {
			continue
		}
		return strings.TrimPrefix(dep.Version, "v")
	}
	return SDKVersion
}
