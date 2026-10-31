package update

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseVersion parses a major.minor.patch string. A leading "v" is ignored.
// Extra pre-release/build suffixes after the numeric triple are dropped.
func ParseVersion(s string) (major, minor, patch int, err error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if s == "" {
		return 0, 0, 0, fmt.Errorf("empty version")
	}
	// Drop -pre / +build for comparison.
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return 0, 0, 0, fmt.Errorf("invalid version %q", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, 0, fmt.Errorf("invalid version %q", s)
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], nil
}

// Compare returns -1 if a < b, 0 if equal, 1 if a > b.
func Compare(a, b string) (int, error) {
	am, an, ap, err := ParseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("local version: %w", err)
	}
	bm, bn, bp, err := ParseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("remote version: %w", err)
	}
	if am != bm {
		if am < bm {
			return -1, nil
		}
		return 1, nil
	}
	if an != bn {
		if an < bn {
			return -1, nil
		}
		return 1, nil
	}
	if ap != bp {
		if ap < bp {
			return -1, nil
		}
		return 1, nil
	}
	return 0, nil
}

// IsNewer reports whether remote is a strictly greater semver than local.
// Invalid versions and local "0.0.0" never count as an update.
func IsNewer(local, remote string) bool {
	lm, ln, lp, err := ParseVersion(local)
	if err != nil {
		return false
	}
	if lm == 0 && ln == 0 && lp == 0 {
		return false
	}
	cmp, err := Compare(local, remote)
	if err != nil {
		return false
	}
	return cmp < 0
}
