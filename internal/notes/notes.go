// Package notes renders categorized Markdown release notes.
package notes

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/faridlamaul/shiptag/internal/commits"
	"github.com/faridlamaul/shiptag/internal/provider"
)

// Section groups commit types under a title.
type Section struct {
	Title  string
	Types  []string
	Hidden bool
}

// Options controls rendering.
type Options struct {
	Heading        string    // tag name, or "Unreleased"
	PreviousTag    string    // "" = no compare link
	CompareTo      string    // ref for the compare URL: the new tag, or a short SHA
	Date           time.Time // rendered as 2006-01-02 in UTC
	BreakingTitle  string
	Sections       []Section
	Other          Section
	ExcludeAuthors []string
	Linker         provider.Linker
}

// Render returns the release notes Markdown, the number of commits rendered,
// and the number of commits left out because their section is hidden.
func Render(cs []commits.Commit, o Options) (markdown string, counted, hidden int) {
	typeSection := map[string]int{}
	for i, s := range o.Sections {
		for _, t := range s.Types {
			if _, ok := typeSection[t]; !ok {
				typeSection[t] = i
			}
		}
	}
	other := len(o.Sections) // index of the Other bucket
	buckets := make([][]commits.Commit, len(o.Sections)+1)
	var breaking []commits.Commit
	for _, c := range cs {
		if c.Breaking {
			breaking = append(breaking, c)
			counted++
			continue
		}
		idx, ok := typeSection[c.Type]
		if !ok {
			idx = other
		}
		hide := o.Other.Hidden
		if idx < other {
			hide = o.Sections[idx].Hidden
		}
		if hide {
			hidden++
			continue
		}
		buckets[idx] = append(buckets[idx], c)
		counted++
	}

	var b strings.Builder
	date := o.Date.UTC().Format("2006-01-02")
	if o.PreviousTag != "" {
		b.WriteString("## [" + o.Heading + "](" + o.Linker.CompareURL(o.PreviousTag, o.CompareTo) + ") (" + date + ")\n\n")
	} else {
		b.WriteString("## " + o.Heading + " (" + date + ")\n\n")
	}

	if counted == 0 {
		b.WriteString("_No user-facing changes._\n\n")
	}
	writeSection(&b, o, o.BreakingTitle, breaking, true)
	for i, s := range o.Sections {
		writeSection(&b, o, s.Title, buckets[i], false)
	}
	writeSection(&b, o, o.Other.Title, buckets[other], false)

	if names := contributors(cs, o); len(names) > 0 {
		b.WriteString("### Contributors\n\n" + strings.Join(names, ", ") + "\n")
	}
	return b.String(), counted, hidden
}

func writeSection(b *strings.Builder, o Options, title string, cs []commits.Commit, isBreaking bool) {
	if len(cs) == 0 {
		return
	}
	b.WriteString("### " + title + "\n\n")
	for _, c := range cs {
		b.WriteString(line(c, o.Linker, isBreaking) + "\n")
	}
	b.WriteString("\n")
}

func line(c commits.Commit, l provider.Linker, isBreaking bool) string {
	text := c.Subject
	if isBreaking && strings.TrimSpace(c.BreakingNote) != "" {
		text = strings.Join(strings.Fields(c.BreakingNote), " ")
	}
	var b strings.Builder
	b.WriteString("- ")
	if c.Scope != "" {
		b.WriteString("**" + escape(c.Scope) + ":** ")
	}
	b.WriteString(escape(text))
	if c.ChangeRequest > 0 {
		b.WriteString(" ([" + l.ChangeRequestRef(c.ChangeRequest) + "](" + l.ChangeRequestURL(c.ChangeRequest) + "))")
	}
	short := c.SHA
	if len(short) > 7 {
		short = short[:7]
	}
	b.WriteString(" ([" + short + "](" + l.CommitURL(c.SHA) + "))")
	for i, n := range c.Issues {
		if i == 0 {
			b.WriteString(", closes ")
		} else {
			b.WriteString(", ")
		}
		b.WriteString("[#" + strconv.Itoa(n) + "](" + l.IssueURL(n) + ")")
	}
	return b.String()
}

var escaper = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, `<`, `&lt;`, `>`, `&gt;`)

func escape(s string) string {
	s = escaper.Replace(s)
	if strings.HasPrefix(s, "#") {
		s = `\` + s
	}
	return s
}

func contributors(cs []commits.Commit, o Options) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p commits.Person) {
		key := strings.ToLower(p.Email)
		if key == "" {
			key = "name:" + strings.ToLower(p.Name)
		}
		if seen[key] {
			return
		}
		seen[key] = true
		name, email := strings.ToLower(p.Name), strings.ToLower(p.Email)
		for _, pat := range o.ExcludeAuthors {
			pat = strings.ToLower(pat)
			if pat != "" && (strings.Contains(name, pat) || strings.Contains(email, pat)) {
				return
			}
		}
		if m := o.Linker.Mention(p.Name, p.Email); m != "" {
			out = append(out, m)
		}
	}
	for _, c := range cs {
		add(c.Author)
		for _, co := range c.CoAuthors {
			add(co)
		}
	}
	sortKey := func(s string) string { return strings.ToLower(strings.TrimPrefix(s, "@")) }
	sort.SliceStable(out, func(i, j int) bool { return sortKey(out[i]) < sortKey(out[j]) })
	return out
}
