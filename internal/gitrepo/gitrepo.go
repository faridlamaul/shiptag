// Package gitrepo reads tags and commits from a local git clone by shelling
// out to the git binary.
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/faridlamaul/shiptag/internal/commits"
)

// ErrNotRepository is returned by Open when dir is not inside a git work tree.
var ErrNotRepository = errors.New("not a git repository")

// Repo is a handle on a local git work tree.
type Repo struct{ Dir string }

// Open verifies git is available and dir is inside a work tree.
func Open(dir string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git executable not found: %w", err)
	}
	r := &Repo{Dir: dir}
	out, err := r.git("rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(out) != "true" {
		return nil, fmt.Errorf("%w: %s", ErrNotRepository, dir)
	}
	return r, nil
}

func (r *Repo) git(args ...string) (string, error) {
	full := append([]string{"-c", "log.showSignature=false", "-c", "color.ui=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (r *Repo) line(args ...string) (string, error) {
	out, err := r.git(args...)
	return strings.TrimSpace(out), err
}

// TopLevel returns the root directory of the work tree.
func (r *Repo) TopLevel() (string, error) { return r.line("rev-parse", "--show-toplevel") }

// ResolveSHA resolves ref to a full commit SHA.
func (r *Repo) ResolveSHA(ref string) (string, error) {
	return r.line("rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
}

// IsShallow reports whether the clone is shallow.
func (r *Repo) IsShallow() (bool, error) {
	out, err := r.line("rev-parse", "--is-shallow-repository")
	return out == "true", err
}

// Tags lists tags starting with prefix; when mergedInto is non-empty only
// tags reachable from it are returned.
func (r *Repo) Tags(prefix, mergedInto string) ([]string, error) {
	args := []string{"tag", "--list"}
	if mergedInto != "" {
		args = append(args, "--merged", mergedInto)
	}
	if prefix != "" {
		args = append(args, prefix+"*")
	}
	out, err := r.git(args...)
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			tags = append(tags, l)
		}
	}
	return tags, nil
}

// Log returns non-merge commits in from..to (or all of to's history when from
// is empty), oldest first, optionally limited to path.
func (r *Repo) Log(from, to, path string) ([]commits.Raw, error) {
	rng := to
	if from != "" {
		rng = from + ".." + to
	}
	args := []string{"log", "--no-merges", "--topo-order", "--reverse", "--format=%H%x1f%an%x1f%ae%x1f%B%x1e", rng}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := r.git(args...)
	if err != nil {
		return nil, err
	}
	var res []commits.Raw
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		parts := strings.SplitN(rec, "\x1f", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("unexpected git log record: %q", rec)
		}
		res = append(res, commits.Raw{
			SHA: parts[0], AuthorName: parts[1], AuthorEmail: parts[2],
			Message: strings.TrimRight(parts[3], "\n"),
		})
	}
	return res, nil
}

// RemoteURL returns the URL of the named remote, or "" if it does not exist.
func (r *Repo) RemoteURL(name string) (string, error) {
	out, err := r.git("remote", "get-url", name)
	if err != nil {
		if strings.Contains(err.Error(), "No such remote") {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}
