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
	if err != nil {
		// Keep git's own explanation: "dubious ownership" or permission
		// problems look like a missing repository otherwise.
		detail := err.Error()
		var ge *gitError
		if errors.As(err, &ge) && ge.stderr != "" {
			detail = ge.stderr
		}
		return nil, fmt.Errorf("%w: %s: %s", ErrNotRepository, dir, detail)
	}
	if strings.TrimSpace(out) != "true" {
		return nil, fmt.Errorf("%w: %s", ErrNotRepository, dir)
	}
	return r, nil
}

// gitError is a failed git invocation with its trimmed stderr.
type gitError struct {
	args   []string
	err    error
	stderr string
}

func (e *gitError) Error() string {
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.args, " "), e.err, e.stderr)
}

func (e *gitError) Unwrap() error { return e.err }

func (r *Repo) git(args ...string) (string, error) {
	full := append([]string{"-c", "log.showSignature=false", "-c", "color.ui=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", &gitError{args: args, err: err, stderr: strings.TrimSpace(stderr.String())}
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
	sha, err := r.line("rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err == nil {
		return sha, nil
	}
	if _, herr := r.line("rev-parse", "--verify", "HEAD"); herr != nil {
		return "", errors.New("repository has no commits")
	}
	return "", fmt.Errorf("cannot resolve ref %q: %w", ref, err)
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

// Merge is a first-parent merge commit and the commits it brought in.
type Merge struct {
	SHA, Message string
	Commits      []string // SHAs reachable from the merged parents but not the first parent
}

// Merges returns the merge commits on the first-parent chain of from..to (or
// all of to's history when from is empty), oldest first. Each merge lists the
// commits in M^1..M^2 (all non-first parents for octopus merges).
//
// path is accepted for symmetry with Log but not applied: git's history
// simplification hides merges that are TREESAME to a parent, which would drop
// exactly the merges that brought path changes in. Callers only use merges to
// annotate commits already filtered by Log, so unrelated merges are harmless.
func (r *Repo) Merges(from, to, path string) ([]Merge, error) {
	_ = path
	rng := to
	if from != "" {
		rng = from + ".." + to
	}
	out, err := r.git("log", "--first-parent", "--merges", "--topo-order", "--reverse", "--format=%H%x1f%B%x1e", rng)
	if err != nil {
		return nil, err
	}
	var res []Merge
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		sha, msg, ok := strings.Cut(rec, "\x1f")
		if !ok {
			return nil, fmt.Errorf("unexpected git log record: %q", rec)
		}
		list, err := r.git("rev-list", sha+"^@", "^"+sha+"^1")
		if err != nil {
			return nil, err
		}
		m := Merge{SHA: sha, Message: strings.TrimRight(msg, "\n")}
		for _, l := range strings.Split(list, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				m.Commits = append(m.Commits, l)
			}
		}
		res = append(res, m)
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
