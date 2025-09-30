package version

import (
	"fmt"
	"runtime"
)

// These values are set during build time
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// Info returns formatted version information
func Info() string {
	return fmt.Sprintf("Version: %s, Commit: %s, Built: %s, Go: %s",
		Version, Commit, BuildDate, runtime.Version())
}

// ShortInfo returns a shorter version string
func ShortInfo() string {
	return Version
}
