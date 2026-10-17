package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/faridlamaul/shiptag/internal/commits"
	ver "github.com/faridlamaul/shiptag/internal/version"
)

// Plan describes a release: what a dry-run would do, or what a real run did.
type Plan struct {
	Provider    string       `json:"provider"`
	Repository  string       `json:"repository"`
	Ref         string       `json:"ref"`
	SHA         string       `json:"sha"`
	PreviousTag string       `json:"previous_tag"`
	NextTag     string       `json:"next_tag"`
	Version     string       `json:"version"`
	Bump        string       `json:"bump"`
	Released    bool         `json:"released"`
	DryRun      bool         `json:"dry_run"`
	URL         string       `json:"url,omitempty"`
	Reasons     []PlanReason `json:"reasons"`
	Commits     struct {
		Counted int `json:"counted"`
		Hidden  int `json:"hidden"`
	} `json:"commits"`
	Actions []string `json:"actions"`
	Notes   string   `json:"notes"`

	reasonLines []string // human-readable Reason lines
}

// PlanReason is a commit that determined the bump.
type PlanReason struct {
	SHA     string `json:"sha"`
	Type    string `json:"type"`
	Scope   string `json:"scope"`
	Subject string `json:"subject"`
	Bump    string `json:"bump"`
}

// newPlan fills the parts of a plan known after analysis.
func newPlan(a *analysis, req request, dryRun bool) *Plan {
	p := &Plan{
		Provider:    a.info.Name,
		Repository:  a.info.Repo,
		Ref:         a.ref,
		SHA:         a.sha,
		PreviousTag: a.previousTag(),
		DryRun:      dryRun,
		Reasons:     []PlanReason{},
		Actions:     []string{},
	}
	if a.release {
		p.NextTag = a.result.Tag
		p.Version = a.result.Version.String()
		p.Bump = a.result.Bump
	}
	for _, r := range a.reasons {
		p.Reasons = append(p.Reasons, PlanReason{
			SHA: r.Commit.SHA, Type: r.Commit.Type, Scope: r.Commit.Scope,
			Subject: r.Commit.Subject, Bump: r.Level.String(),
		})
	}

	switch {
	case !a.release:
	case a.result.Bump == "explicit":
		p.reasonLines = []string{"explicit ← --version " + req.explicit}
	case req.bump != "" && req.bump != "auto":
		p.reasonLines = []string{a.result.Bump + " ← --bump " + req.bump}
	case a.result.Bump == "initial":
		p.reasonLines = []string{"initial ← no previous version tag (initial_version " + a.cfg.InitialVersion + ")"}
	default:
		for _, r := range a.reasons {
			p.reasonLines = append(p.reasonLines, reasonLine(r))
		}
	}
	return p
}

// reasonLine renders "<level> ← <type>(<scope>)!: <subject> (<sha7>)".
func reasonLine(r ver.Reason) string {
	return r.Level.String() + " ← " + header(r.Commit) + " (" + short(r.Commit.SHA, 7) + ")"
}

func header(c commits.Commit) string {
	h := c.Type
	if c.Scope != "" {
		h += "(" + c.Scope + ")"
	}
	if c.Breaking {
		h += "!"
	}
	return h + ": " + c.Subject
}

// writeHuman prints the plan in the layout of spec section 7.
func (p *Plan) writeHuman(w io.Writer) error {
	var b strings.Builder
	prev := p.PreviousTag
	if prev == "" {
		prev = "(none)"
	}
	fmt.Fprintf(&b, "Provider:     %s (%s)\n", p.Provider, p.Repository)
	fmt.Fprintf(&b, "Ref:          %s → %s…\n", p.Ref, short(p.SHA, 8))
	fmt.Fprintf(&b, "Previous tag: %s\n", prev)
	fmt.Fprintf(&b, "Next tag:     %s (%s)\n", p.NextTag, p.Bump)
	for i, l := range p.reasonLines {
		if i == 0 {
			b.WriteString("Reason:       " + l + "\n")
		} else {
			b.WriteString(strings.Repeat(" ", 14) + l + "\n")
		}
	}
	if len(p.reasonLines) == 0 {
		b.WriteString("Reason:       (none)\n")
	}
	fmt.Fprintf(&b, "Commits:      %d counted, %d hidden\n", p.Commits.Counted, p.Commits.Hidden)
	b.WriteString("Actions:\n")
	for _, a := range p.Actions {
		b.WriteString("  - " + a + "\n")
	}
	b.WriteString("\n--- Release notes ---\n")
	b.WriteString(p.Notes)
	_, err := io.WriteString(w, b.String())
	return err
}

// writeJSON prints the plan as one JSON object.
func (p *Plan) writeJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}

// publishAction describes publishing tag at sha for a provider; dry selects
// the "would …" form.
func publishAction(providerName, tag, sha string, draft, dry bool) string {
	s7 := short(sha, 7)
	if dry {
		switch providerName {
		case "gitlab":
			return fmt.Sprintf("would create release %s at %s (tag created by release)", tag, s7)
		case "bitbucket":
			return fmt.Sprintf("would create annotated tag %s at %s", tag, s7)
		}
		return fmt.Sprintf("would create release %s at %s (draft=%t)", tag, s7, draft)
	}
	switch providerName {
	case "gitlab":
		return fmt.Sprintf("created release %s at %s (tag created by release)", tag, s7)
	case "bitbucket":
		return fmt.Sprintf("created annotated tag %s at %s", tag, s7)
	}
	return fmt.Sprintf("created release %s at %s (draft=%t)", tag, s7, draft)
}
