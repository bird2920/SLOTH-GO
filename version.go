package main

import "runtime/debug"

// devVersion marks a build that the Makefile did not stamp.
const devVersion = "dev"

// version is stamped at build time by the Makefile (-ldflags "-X main.version=...").
var version = devVersion

// buildVersion returns the stamped version, falling back to the git revision Go embeds
// in builds made from a checkout without the Makefile.
func buildVersion() string {
	if version != devVersion {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return version
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "-dirty"
	}
	return devVersion + "-" + rev
}
