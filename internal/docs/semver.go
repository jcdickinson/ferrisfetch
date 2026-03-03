package docs

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed semver version.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease string
}

// ParseVersion parses a semver version string like "1.2.3" or "1.2.3-beta.1".
func ParseVersion(s string) (Version, error) {
	s = strings.TrimPrefix(s, "v")

	pre := ""
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
	}

	// Strip build metadata
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Version{}, fmt.Errorf("invalid version: %q", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch := 0
	if len(parts) == 3 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("invalid patch version: %w", err)
		}
	}

	return Version{Major: major, Minor: minor, Patch: patch, PreRelease: pre}, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.PreRelease != "" {
		s += "-" + v.PreRelease
	}
	return s
}

// Compare returns -1, 0, or 1 comparing v to other.
// Pre-release versions sort before their release (1.0.0-alpha < 1.0.0).
func (v Version) Compare(other Version) int {
	if c := cmpInt(v.Major, other.Major); c != 0 {
		return c
	}
	if c := cmpInt(v.Minor, other.Minor); c != 0 {
		return c
	}
	if c := cmpInt(v.Patch, other.Patch); c != 0 {
		return c
	}
	// No pre-release > has pre-release
	if v.PreRelease == "" && other.PreRelease != "" {
		return 1
	}
	if v.PreRelease != "" && other.PreRelease == "" {
		return -1
	}
	if v.PreRelease < other.PreRelease {
		return -1
	}
	if v.PreRelease > other.PreRelease {
		return 1
	}
	return 0
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// SatisfiesReq checks if a version satisfies a Cargo-flavored version requirement.
// Supports: ^X.Y.Z, ~X.Y.Z, =X.Y.Z, >=, <=, >, <, bare X.Y.Z (treated as ^X.Y.Z),
// and compound ranges separated by ", ".
func SatisfiesReq(version, req string) bool {
	req = strings.TrimSpace(req)
	if req == "" || req == "*" {
		return true
	}

	// Compound requirements: all must match
	if strings.Contains(req, ",") {
		for part := range strings.SplitSeq(req, ",") {
			if !SatisfiesReq(version, strings.TrimSpace(part)) {
				return false
			}
		}
		return true
	}

	ver, err := ParseVersion(version)
	if err != nil {
		return false
	}

	// Pre-release versions only match exact requirements in Cargo
	if ver.PreRelease != "" {
		return satisfiesPreRelease(ver, req)
	}

	switch {
	case strings.HasPrefix(req, ">="):
		r, err := ParseVersion(strings.TrimPrefix(req, ">="))
		if err != nil {
			return false
		}
		return ver.Compare(r) >= 0
	case strings.HasPrefix(req, "<="):
		r, err := ParseVersion(strings.TrimPrefix(req, "<="))
		if err != nil {
			return false
		}
		return ver.Compare(r) <= 0
	case strings.HasPrefix(req, ">"):
		r, err := ParseVersion(strings.TrimPrefix(req, ">"))
		if err != nil {
			return false
		}
		return ver.Compare(r) > 0
	case strings.HasPrefix(req, "<"):
		r, err := ParseVersion(strings.TrimPrefix(req, "<"))
		if err != nil {
			return false
		}
		return ver.Compare(r) < 0
	case strings.HasPrefix(req, "="):
		r, err := ParseVersion(strings.TrimPrefix(req, "="))
		if err != nil {
			return false
		}
		return ver.Compare(r) == 0
	case strings.HasPrefix(req, "~"):
		return satisfiesTilde(ver, strings.TrimPrefix(req, "~"))
	case strings.HasPrefix(req, "^"):
		return satisfiesCaret(ver, strings.TrimPrefix(req, "^"))
	default:
		// Bare version: treated as ^version
		return satisfiesCaret(ver, req)
	}
}

// satisfiesCaret implements Cargo's ^ requirement:
//
//	^X.Y.Z (X>0): >=X.Y.Z, <(X+1).0.0
//	^0.Y.Z (Y>0): >=0.Y.Z, <0.(Y+1).0
//	^0.0.Z:       >=0.0.Z, <0.0.(Z+1)
func satisfiesCaret(ver Version, reqStr string) bool {
	r, err := ParseVersion(reqStr)
	if err != nil {
		return false
	}
	if ver.Compare(r) < 0 {
		return false
	}

	var upper Version
	switch {
	case r.Major > 0:
		upper = Version{Major: r.Major + 1}
	case r.Minor > 0:
		upper = Version{Minor: r.Minor + 1}
	default:
		upper = Version{Patch: r.Patch + 1}
	}
	return ver.Compare(upper) < 0
}

// satisfiesTilde implements ~X.Y.Z: >=X.Y.Z, <X.(Y+1).0
func satisfiesTilde(ver Version, reqStr string) bool {
	r, err := ParseVersion(reqStr)
	if err != nil {
		return false
	}
	if ver.Compare(r) < 0 {
		return false
	}
	upper := Version{Major: r.Major, Minor: r.Minor + 1}
	return ver.Compare(upper) < 0
}

// satisfiesPreRelease handles pre-release version matching.
// In Cargo, a pre-release only matches if the requirement targets
// the same major.minor.patch.
func satisfiesPreRelease(ver Version, req string) bool {
	// Strip operator prefix
	op := ""
	reqStr := req
	for _, prefix := range []string{">=", "<=", "^", "~", "=", ">", "<"} {
		if after, ok := strings.CutPrefix(reqStr, prefix); ok {
			op = prefix
			reqStr = after
			break
		}
	}

	r, err := ParseVersion(reqStr)
	if err != nil {
		return false
	}

	// Pre-release only matches same major.minor.patch base
	if ver.Major != r.Major || ver.Minor != r.Minor || ver.Patch != r.Patch {
		return false
	}

	// If req also has a pre-release, compare directly
	if r.PreRelease != "" {
		cmp := ver.Compare(r)
		switch op {
		case ">=":
			return cmp >= 0
		case "<=":
			return cmp <= 0
		case ">":
			return cmp > 0
		case "<":
			return cmp < 0
		case "=":
			return cmp == 0
		default:
			// ^, ~, or bare: pre-release >= req pre-release on same base
			return cmp >= 0
		}
	}

	// Bare requirement for same base: pre-release satisfies
	return true
}
