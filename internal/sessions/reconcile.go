package sessions

import (
	"github.com/MTG-Thomas/spuro/internal/model"
	"sort"
)

// Reconciliation is a presentation partition, not another lifecycle verdict.
// Raw assessments, evidence, and candidates remain in the JSON envelope.
type Reconciliation struct {
	Review     []ReconciliationItem `json:"review"`
	Candidates []ReconciliationItem `json:"candidates"`
	RuledDown  []ReconciliationItem `json:"ruled_down"`
}

type ReconciliationItem struct {
	SessionID         string              `json:"session_id"`
	IntentID          string              `json:"intent_id"`
	State             model.IntentState   `json:"state"`
	Confidence        model.Confidence    `json:"confidence"`
	Coverage          model.CoverageLevel `json:"later_session_coverage"`
	Description       string              `json:"description,omitempty"`
	Reasons           []string            `json:"reasons"`
	SatisfiedCriteria []string            `json:"satisfied_criteria"`
	Evidence          []string            `json:"evidence"`
	RelatedFindings   []string            `json:"related_findings"`
}

func Reconcile(r *Result) Reconciliation {
	t := Reconciliation{Review: []ReconciliationItem{}, Candidates: []ReconciliationItem{}, RuledDown: []ReconciliationItem{}}
	descriptions := map[string]string{}
	for _, s := range r.Sessions {
		for _, i := range s.Intents {
			descriptions[s.ID+"\x00"+i.ID] = i.Description
		}
	}
	for _, s := range r.Assessments {
		for _, a := range s.Intents {
			item := ReconciliationItem{SessionID: s.SessionID, IntentID: a.IntentID, State: a.State, Confidence: a.Confidence, Coverage: a.Coverage.SessionHistory.Level, Description: descriptions[s.SessionID+"\x00"+a.IntentID], Reasons: a.Reasons, SatisfiedCriteria: a.SatisfiedCriteria, Evidence: a.Evidence, RelatedFindings: a.RelatedFindings}
			switch a.State {
			case model.IntentDirtyStateRemains, model.IntentFailedUnresolved, model.IntentUnresolved:
				t.Review = append(t.Review, item)
			case model.IntentCompleted, model.IntentSuperseded, model.IntentResumedLater:
				t.RuledDown = append(t.RuledDown, item)
			default:
				t.Candidates = append(t.Candidates, item)
			}
		}
	}
	sort.SliceStable(t.Review, func(i, j int) bool { return priority(t.Review[i].State) < priority(t.Review[j].State) })
	return t
}

func priority(state model.IntentState) int {
	switch state {
	case model.IntentDirtyStateRemains:
		return 0
	case model.IntentFailedUnresolved:
		return 1
	default:
		return 2
	}
}
