// Package gitrepo reads tags and commits from a local git clone by shelling
// out to the git binary.
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
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
		// After "--" so a prefix such as "-n" is a pattern, not an option.
		args = append(args, "--", prefix+"*")
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
	if spec := pathspec(path); spec != "" {
		args = append(args, "--", spec)
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

// pathspec turns a repository-root relative path into a pathspec git resolves
// from the top of the work tree regardless of Repo.Dir. "" and "." (with any
// leading "./" or "/") mean the whole repository and yield "".
func pathspec(path string) string {
	for {
		switch {
		case strings.HasPrefix(path, "./"):
			path = path[2:]
		case strings.HasPrefix(path, "/"):
			path = path[1:]
		default:
			if path == "" || path == "." {
				return ""
			}
			return ":(top)" + path
		}
	}
}

// Merge is a first-parent merge commit and the commits it brought in.
type Merge struct {
	SHA, Message string
	Commits      []string // SHAs reachable from the merged parents but not the first parent
}

// Merges returns the merge commits on the first-parent chain of from..to (or
// all of to's history when from is empty), oldest first. Each merge lists the
// commits in M^1..M^2 (all non-first parents for octopus merges) that are
// also in the range. The whole range costs two git calls, not one per merge.
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
	introduced, err := r.mergeCommits(rng)
	if err != nil {
		return nil, err
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
		res = append(res, Merge{SHA: sha, Message: strings.TrimRight(msg, "\n"), Commits: introduced[sha]})
	}
	return res, nil
}

// mergeCommits reads the commit graph of rng once and returns, for every
// first-parent merge in it, the in-range commits it introduced: those
// reachable from its non-first parents but not from its first parent. This
// equals `git rev-list M^@ ^M^1` limited to rng, without a git process per
// merge. Commits are listed in rev-list --topo-order order.
func (r *Repo) mergeCommits(rng string) (map[string][]string, error) {
	out, err := r.git("rev-list", "--parents", "--topo-order", "--end-of-options", rng)
	if err != nil {
		return nil, err
	}
	parents := map[string][]string{} // every commit in range
	order := map[string]int{}        // position in rev-list output
	var tip string
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		if tip == "" {
			tip = f[0]
		}
		order[f[0]] = len(order)
		parents[f[0]] = f[1:]
	}
	if tip == "" {
		return nil, nil
	}
	// --topo-order lists the tip (to) first; walk its first-parent chain while it
	// stays in range, then replay it oldest first.
	var chain []string
	for c := tip; ; {
		ps, ok := parents[c]
		if !ok {
			break
		}
		chain = append(chain, c)
		if len(ps) == 0 {
			break
		}
		c = ps[0]
	}
	// seen holds the in-range ancestors of the chain commit being processed's
	// first parent; nothing in range reaches them except through it.
	seen := map[string]bool{}
	res := map[string][]string{}
	for i := len(chain) - 1; i >= 0; i-- {
		m := chain[i]
		var introduced []string
		queue := append([]string(nil), parents[m][min(1, len(parents[m])):]...)
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			if _, in := parents[c]; !in || seen[c] {
				continue
			}
			seen[c] = true
			introduced = append(introduced, c)
			queue = append(queue, parents[c]...)
		}
		seen[m] = true
		if len(parents[m]) >= 2 && len(introduced) > 0 {
			sort.Slice(introduced, func(a, b int) bool { return order[introduced[a]] < order[introduced[b]] })
			res[m] = introduced
		}
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
