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
	for si := range r.Sessions {
		s := &r.Sessions[si]
		for ii := range s.Intents {
			i := &s.Intents[ii]
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
	counts := map[model.Severity]int{}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	fmt.Fprintf(w, "PRESERVE FIRST %d; REVIEW %d; INFO %d\n", counts[model.PreserveFirst], counts[model.Review], counts[model.Info])
	descriptions := map[string]string{}
	for _, s := range r.Sessions {
		for _, i := range s.Intents {
			descriptions[s.ID+"\x00"+i.ID] = i.Description
		}
	}
	for _, a := range r.Assessments {
		if !includeCompleted {
			visible := false
			for _, i := range a.Intents {
				if i.State != model.IntentCompleted && i.State != model.IntentSuperseded && i.State != model.IntentResumedLater {
					visible = true
				}
			}
			if !visible {
				continue
			}
		}
		if a.Summary == model.SessionCompleted && !includeCompleted {
			continue
		}
		fmt.Fprintf(w, "%s: %s\n", a.SessionID, a.Summary)
		for _, i := range a.Intents {
			if !includeCompleted && (i.State == model.IntentCompleted || i.State == model.IntentSuperseded || i.State == model.IntentResumedLater) {
				continue
			}
			fmt.Fprintf(w, "  %s: %s (confidence %s; later coverage %s)\n", i.IntentID, i.State, i.Confidence, i.Coverage.SessionHistory.Level)
			if description := descriptions[a.SessionID+"\x00"+i.IntentID]; description != "" {
				fmt.Fprintf(w, "    %s\n", description)
			}
			for _, reason := range i.Reasons {
				fmt.Fprintf(w, "    %s\n", reason)
			}
		}
	}
	for _, d := range r.Diagnostics {
		fmt.Fprintf(w, "diagnostic: %s\n", d.Message)
	}
	_, err := fmt.Fprintln(w, "No provider commands executed. Additional normal-ref copies are not verified backups. No abandonment, authorship, squash, or semantic-completion claim.")
	return err
}
