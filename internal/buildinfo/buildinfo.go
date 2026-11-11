// Package buildinfo reports which build of sentinel is running.
package buildinfo

import (
	"runtime"
	"runtime/debug"
)

// version is set at release time with
// -ldflags "-X github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo.version=v1.2.3".
// A build without it is a development build.
var version = "dev"

// Info describes a build.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

// Read returns the build information of the running binary.
func Read() Info {
	info := Info{Version: version, Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				info.Commit = s.Value
			}
		}
	}
	return info
}
