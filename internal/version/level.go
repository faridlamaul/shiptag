// Package version computes semantic versions from commit history.
package version

import (
	"fmt"

	"github.com/faridlamaul/shiptag/internal/commits"
)

// Level is a bump level; higher values win.
type Level int

// Bump levels in ascending order.
const (
	None Level = iota
	Patch
	Minor
	Major
)

var levelNames = [...]string{"none", "patch", "minor", "major"}

// ParseLevel converts "none", "patch", "minor" or "major" to a Level.
func ParseLevel(s string) (Level, error) {
	for i, n := range levelNames {
		if n == s {
			return Level(i), nil
		}
	}
	return None, fmt.Errorf("unknown bump level %q", s)
}

// String returns the level name.
func (l Level) String() string {
	if l < None || l > Major {
		return "unknown"
	}
	return levelNames[l]
}

// Rules maps commit types to bump levels.
type Rules struct{ Types map[string]Level }

// Reason is a commit that contributed to the winning level.
type Reason struct {
	Commit commits.Commit
	Level  Level
}

// Determine returns the highest level and the commits that reached it, in input order.
// Breaking commits are always Major.
func Determine(cs []commits.Commit, r Rules) (Level, []Reason) {
	levels := make([]Level, len(cs))
	highest := None
	for i, c := range cs {
		l := r.Types[c.Type]
		if c.Breaking {
			l = Major
		}
		levels[i] = l
		if l > highest {
			highest = l
		}
	}
	var reasons []Reason
	if highest == None {
		return None, nil
	}
	for i, c := range cs {
		if levels[i] == highest {
			reasons = append(reasons, Reason{Commit: c, Level: highest})
		}
	}
	return highest, reasons
}
