// Package output writes CI outputs and prepends release notes to a changelog.
package output

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// rename is a seam for tests.
var rename = os.Rename

// Values are the release results exposed to CI.
type Values struct {
	Released                                      bool
	Version, PreviousVersion, Tag, SHA, Bump, URL string
}

// Lines returns key=value lines in a stable order.
func (v Values) Lines() []string {
	pairs := [][2]string{
		{"version", v.Version},
		{"previous_version", v.PreviousVersion},
		{"tag", v.Tag},
		{"sha", v.SHA},
		{"bump", v.Bump},
		{"url", v.URL},
	}
	lines := make([]string, 0, len(pairs)+1)
	lines = append(lines, fmt.Sprintf("released=%t", v.Released))
	for _, p := range pairs {
		lines = append(lines, p[0]+"="+sanitize(p[1]))
	}
	return lines
}

// Destination returns the file to append outputs to, or "" for stdout.
func Destination(providerName string, inCI bool, override string, getenv func(string) string) string {
	if override != "" {
		return override
	}
	if !inCI {
		return ""
	}
	switch providerName {
	case "github":
		return getenv("GITHUB_OUTPUT")
	case "gitlab", "bitbucket":
		return "shiptag.env"
	}
	return ""
}

// Write appends the output lines to dest, or writes them to stdout when dest is "".
func Write(dest string, v Values, stdout io.Writer) error {
	var sb strings.Builder
	for _, l := range v.Lines() {
		sb.WriteString(l + "\n")
	}
	if dest == "" {
		_, err := io.WriteString(stdout, sb.String())
		return err
	}
	f, err := os.OpenFile(dest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(sb.String()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func sanitize(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// PrependChangelog inserts notes at the top of the changelog (below a leading
// "# " heading, if any), creating the file when missing. Existing file mode and
// CRLF line endings are preserved.
func PrependChangelog(path, notes string) error {
	mode := fs.FileMode(0o644)
	crlf := false
	existing, err := os.ReadFile(path)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	if !missing {
		if st, serr := os.Stat(path); serr == nil {
			mode = st.Mode().Perm()
		}
		crlf = strings.Contains(string(existing), "\r\n")
	}
	notes = strings.ReplaceAll(notes, "\r\n", "\n")
	if !strings.HasSuffix(notes, "\n") {
		notes += "\n"
	}
	var out string
	if missing {
		out = "# Changelog\n\n" + notes
	} else {
		content := string(existing)
		if crlf {
			content = strings.ReplaceAll(content, "\r\n", "\n")
		}
		first, rest, _ := strings.Cut(content, "\n")
		rest = strings.TrimLeft(rest, "\n")
		switch {
		case strings.HasPrefix(first, "# "):
			out = first + "\n\n" + notes
			if rest != "" {
				out += "\n" + rest
			}
		case content == "":
			out = notes
		default:
			out = notes + "\n" + strings.TrimLeft(content, "\n")
		}
	}
	if crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
