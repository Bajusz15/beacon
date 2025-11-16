package version

// Version information - injected at build time
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	BuildUser = "unknown"
)

// GetVersion returns the version string
func GetVersion() string {
	return Version
}

// GetCommit returns the commit hash
func GetCommit() string {
	return Commit
}

// GetBuildDate returns the build date
func GetBuildDate() string {
	return BuildDate
}

// GetBuildUser returns the build user
func GetBuildUser() string {
	return BuildUser
}

// GetFullVersion returns a formatted version string
func GetFullVersion() string {
	return Version + " (" + Commit + ")"
}

// GetBuildInfo returns all build information
func GetBuildInfo() map[string]string {
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_date": BuildDate,
		"build_user": BuildUser,
	}
}
