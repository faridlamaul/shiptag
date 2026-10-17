// Package commits parses git commit messages following the Conventional
// Commits specification.
package commits

import (
	"regexp"
	"strconv"
	"strings"
)

// Raw is a commit as read from git, before parsing.
type Raw struct{ SHA, AuthorName, AuthorEmail, Message string }

// Person is a commit author or co-author.
type Person struct{ Name, Email string }

// Commit is a parsed commit.
type Commit struct {
	SHA, Type, Scope, Subject string // Type lowercased; "other" when non-conventional; "revert" for git reverts
	Breaking                  bool
	BreakingNote              string // footer text; "" when only "!" was used
	ChangeRequest             int    // 0 when none
	Issues                    []int  // closing references from the body, in order, deduplicated
	Author                    Person
	CoAuthors                 []Person
}

// CRExtractor finds a change-request number and returns the header with the
// reference removed. An extractor may instead return the first non-empty body
// line (see FirstLine) as the cleaned header, for messages whose title is a
// generated "merged pull request" line and whose real header is in the body
// (Bitbucket squash merges); Parse then removes that line from the body.
type CRExtractor func(header, body string) (n int, cleanedHeader string, ok bool)

// FirstLine returns the first non-empty line of body, trimmed.
func FirstLine(body string) string {
	first, _ := splitFirstLine(body)
	return first
}

// splitFirstLine returns the first non-empty line of body (trimmed) and the
// text after it.
func splitFirstLine(body string) (first, rest string) {
	for body != "" {
		line, after, _ := strings.Cut(body, "\n")
		if t := strings.TrimSpace(line); t != "" {
			return t, after
		}
		body = after
	}
	return "", ""
}

var (
	headerRe   = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*)(?:\(([^)]*)\))?(!)?:\s+(.+)$`)
	revertRe   = regexp.MustCompile(`^Revert "(.+)"$`)
	breakingRe = regexp.MustCompile(`^BREAKING[ -]CHANGE:\s*(.*)$`)
	trailerRe  = regexp.MustCompile(`^[A-Za-z-]+(: | #)`)
	issueRe    = regexp.MustCompile(`(?i)(?:^|\s)(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)\b`)
	coAuthorRe = regexp.MustCompile(`(?mi)^co-authored-by:\s*(.+?)\s*<([^>]+)>\s*$`)
)

// Parse parses a raw commit. extract may be nil.
func Parse(r Raw, extract CRExtractor) Commit {
	msg := strings.ReplaceAll(r.Message, "\r\n", "\n")
	header, body, _ := strings.Cut(msg, "\n")
	header = strings.TrimSpace(header)

	c := Commit{
		SHA:    r.SHA,
		Author: Person{Name: r.AuthorName, Email: r.AuthorEmail},
	}

	if extract != nil {
		if n, cleaned, ok := extract(header, body); ok {
			c.ChangeRequest = n
			cleaned = strings.TrimSpace(cleaned)
			if first, rest := splitFirstLine(body); cleaned != header && first != "" && cleaned == first {
				body = rest // the header was taken from the body
			}
			header = cleaned
		}
	}

	switch {
	case revertRe.MatchString(header):
		c.Type = "revert"
		c.Subject = revertRe.FindStringSubmatch(header)[1]
	case headerRe.MatchString(header):
		m := headerRe.FindStringSubmatch(header)
		c.Type = strings.ToLower(m[1])
		c.Scope = m[2]
		c.Breaking = m[3] == "!"
		c.Subject = strings.TrimSpace(m[4])
	default:
		c.Type = "other"
		c.Subject = header
	}

	if note, ok := breakingNote(body); ok {
		c.Breaking = true
		c.BreakingNote = note
	}

	seen := map[int]bool{}
	for _, m := range issueRe.FindAllStringSubmatch(body, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil || seen[n] {
			continue
		}
		seen[n] = true
		c.Issues = append(c.Issues, n)
	}

	for _, m := range coAuthorRe.FindAllStringSubmatch(body, -1) {
		c.CoAuthors = append(c.CoAuthors, Person{Name: m[1], Email: m[2]})
	}

	return c
}

// breakingNote extracts the BREAKING CHANGE footer text. The footer runs until
// a blank line or the next trailer.
func breakingNote(body string) (string, bool) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		m := breakingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		note := []string{m[1]}
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" || trailerRe.MatchString(next) {
				break
			}
			note = append(note, next)
		}
		return strings.TrimSpace(strings.Join(note, "\n")), true
	}
	return "", false
}
