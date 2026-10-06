package sessions

import (
	"encoding/json"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/model"
	"io"
)

// Sanitize mutates only the report in memory, never any artifact. Text-free mode
// removes provider prose and command/literal bodies after deterministic matching.
func Sanitize(r *Result, noText bool) {
	if r.UnknownQueue != nil {
		for n := range r.UnknownQueue.Groups {
			g := &r.UnknownQueue.Groups[n]
			for n := range g.Matches {
				sanitizeProvenance(g.Matches[n].Provenance)
				if g.Matches[n].StashArtifact != nil {
					sanitizeProvenance(&g.Matches[n].StashArtifact.Provenance)
				}
			}
			g.Description = Redact(g.Description)
			if noText {
				g.Description = ""
			}
		}
	}
	for si := range r.Sessions {
		s := &r.Sessions[si]
		if s.Inheritance != nil {
			for n := range s.Inheritance.Forks {
				sanitizeProvenance(&s.Inheritance.Forks[n].Provenance)
			}
			for n := range s.Inheritance.Events {
				sanitizeProvenance(&s.Inheritance.Events[n].Provenance)
			}
			for n := range s.Inheritance.Occurrences {
				sanitizeProvenance(&s.Inheritance.Occurrences[n].Provenance)
			}
			for n := range s.Inheritance.Fragments {
				sanitizeProvenance(&s.Inheritance.Fragments[n].Provenance)
			}
		}
		if s.Context != nil {
			for n := range s.Context.ScopedReverts {
				w := &s.Context.ScopedReverts[n]
				sanitizeProvenance(&w.Event.Provenance)
				w.Request = Redact(w.Request)
				if noText {
					w.Request = ""
				}
			}
			for n := range s.Context.StashArtifacts {
				sanitizeProvenance(&s.Context.StashArtifacts[n].Provenance)
			}
			for n := range s.Context.BoundedRequests {
				sanitizeProvenance(&s.Context.BoundedRequests[n].Provenance)
			}
			for n := range s.Context.TrackerWitnesses {
				sanitizeProvenance(&s.Context.TrackerWitnesses[n].Provenance)
				s.Context.TrackerWitnesses[n].CheckedBy = Redact(s.Context.TrackerWitnesses[n].CheckedBy)
			}
		}
		for ii := range s.Intents {
			i := &s.Intents[ii]
			if i.SourceEvent != nil {
				sanitizeProvenance(&i.SourceEvent.Provenance)
			}
			if i.TargetResolution != nil {
				sanitizeProvenance(&i.TargetResolution.Provenance)
			}
			i.Description = Redact(i.Description)
			if noText {
				i.Description = ""
			}
			for gi := range i.Completion {
				for ci := range i.Completion[gi].Criteria {
					c := &i.Completion[gi].Criteria[ci]
					c.Description = Redact(c.Description)
					c.Literal = Redact(c.Literal)
					for n := range c.Commands {
						c.Commands[n] = Redact(c.Commands[n])
					}
					if noText {
						c.Description = ""
						c.Literal = ""
						c.Commands = nil
					}
				}
			}
			for n := range i.Commands {
				i.Commands[n] = Redact(i.Commands[n])
			}
			if noText {
				i.Commands = nil
			}
		}
		for n := range s.Commands {
			s.Commands[n].Command = Redact(s.Commands[n].Command)
			if noText {
				s.Commands[n].Command = ""
			}
		}
	}
	for n := range r.Findings {
		r.Findings[n].Summary = Redact(r.Findings[n].Summary)
		if noText {
			r.Findings[n].Summary = r.Findings[n].Kind
		}
	}
	for n := range r.Diagnostics {
		r.Diagnostics[n].Message = Redact(r.Diagnostics[n].Message)
	}
}
func JSON(w io.Writer, r *Result) error {
	if r.Format == "" {
		r.Format = "spuro-sessions"
	}
	t := Reconcile(r)
	r.Reconciliation = &t
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
func Text(w io.Writer, r *Result, includeCompleted bool) error {
	if _, err := fmt.Fprintf(w, "Spuro session archaeology — read-only\n%d sessions ingested; %d existing artifacts\n", len(r.Sessions), len(r.Sources)); err != nil {
		return err
	}
	for _, src := range r.Sources {
		fmt.Fprintf(w, "%s source: %q\n  artifact mtime: %s; provider coverage PARTIAL; not refreshed\n", src.Provider, src.Path, src.Modified.Format("2006-01-02T15:04:05Z07:00"))
	}
	triage := Reconcile(r)
	fmt.Fprintf(w, "Substantiated review %d; unverified candidates %d; ruled down %d\n", len(triage.Review), len(triage.Candidates), len(triage.RuledDown))
	fmt.Fprintln(w, "Candidate counts are not forgotten-task counts. Checkout association supplies context, not proof of the intended repository.")
	items := triage.Review
	if includeCompleted {
		items = append(append(append([]ReconciliationItem{}, items...), triage.Candidates...), triage.RuledDown...)
	}
	limit := len(items)
	if !includeCompleted && limit > 20 {
		limit = 20
	}
	for _, item := range items[:limit] {
		fmt.Fprintf(w, "%s / %s: %s (confidence %s; later coverage %s)\n", item.SessionID, item.IntentID, item.State, item.Confidence, item.Coverage)
		if item.Description != "" {
			fmt.Fprintf(w, "  %s\n", item.Description)
		}
		for _, reason := range item.Reasons {
			fmt.Fprintf(w, "  %s\n", reason)
		}
		if len(item.SatisfiedCriteria) > 0 {
			fmt.Fprintf(w, "  Counter-evidence: observed criteria %v\n", item.SatisfiedCriteria)
		}
	}
	if limit < len(items) {
		fmt.Fprintf(w, "%d further substantiated items retained in JSON.\n", len(items)-limit)
	}
	if !includeCompleted && len(triage.Candidates) > 0 {
		fmt.Fprintln(w, "Unverified candidates retained in JSON; use --include-completed for the full reconciliation listing.")
	}
	if r.UnknownQueue != nil {
		if err := UnknownText(w, *r.UnknownQueue); err != nil {
			return err
		}
	}
	for _, d := range r.Diagnostics {
		fmt.Fprintf(w, "diagnostic: %s\n", d.Message)
	}
	_, err := fmt.Fprintln(w, "No provider commands executed. Additional normal-ref copies are not verified backups. No abandonment, authorship, squash, or semantic-completion claim.")
	return err
}

func sanitizeProvenance(p *model.ContextProvenance) {
	if p == nil {
		return
	}
	p.Artifact = Redact(p.Artifact)
	p.Provider = Redact(p.Provider)
}
