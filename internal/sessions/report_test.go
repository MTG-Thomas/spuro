package sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/model"
	"strings"
	"testing"
)

func TestSessionJSONAndTextFreeReport(t *testing.T) {
	criterion := model.CompletionCriterion{Description: "private prose", Literal: "private code", Commands: []string{"command secret"}}
	intent := model.Intent{ID: "docs", Description: "Next: token=secretvalue", Completion: []model.CriterionGroup{{Mode: "ALL", Criteria: []model.CompletionCriterion{criterion}}}}
	session := model.Session{ID: "fixture", Intents: []model.Intent{intent}, Commands: []model.SessionCommand{{Command: "dangerous command"}}}
	r := Result{SchemaVersion: 1, Sessions: []model.Session{session}, Findings: []model.Finding{{Kind: "session_unresolved", Summary: "private prose"}}}
	Sanitize(&r, true)
	var b bytes.Buffer
	if err := JSON(&b, &r); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secretvalue", "private prose", "private code", "dangerous command", "command secret"} {
		if strings.Contains(b.String(), secret) {
			t.Fatal("text-free report leaked content")
		}
	}
	var got Result
	if err := json.Unmarshal(b.Bytes(), &got); err != nil || got.SchemaVersion != 1 {
		t.Fatal("invalid session JSON")
	}
	b.Reset()
	if err := Text(&b, &r, false); err != nil || !strings.Contains(b.String(), "No provider commands executed") {
		t.Fatal("missing safety scope")
	}
}
func TestSummaryMixedAndPartial(t *testing.T) {
	tests := []struct {
		states []model.IntentState
		want   model.SessionSummary
	}{
		{nil, model.SessionUnknown}, {[]model.IntentState{model.IntentCompleted, model.IntentSuperseded}, model.SessionCompleted},
		{[]model.IntentState{model.IntentCompleted, model.IntentUnresolved}, model.SessionPartiallyCompleted},
		{[]model.IntentState{model.IntentUnknown, model.IntentFailedUnresolved}, model.SessionMixed},
		{[]model.IntentState{model.IntentUnknown}, model.SessionUnknown},
	}
	for _, tt := range tests {
		a := []model.IntentAssessment{}
		for _, state := range tt.states {
			a = append(a, model.IntentAssessment{State: state})
		}
		if summary(a) != tt.want {
			t.Fatal("incorrect per-intent summary")
		}
	}
}

func TestReconciliationKeepsUnknownOutOfDefaultShortlist(t *testing.T) {
	r := Result{SchemaVersion: 1}
	for n := 0; n < 2000; n++ {
		id := fmt.Sprintf("candidate-%d", n)
		r.Assessments = append(r.Assessments, model.DerivedSessionAssessment{SessionID: id, Intents: []model.IntentAssessment{{IntentID: "next", State: model.IntentUnknown}}})
	}
	r.Assessments = append(r.Assessments, model.DerivedSessionAssessment{SessionID: "dirty", Intents: []model.IntentAssessment{{IntentID: "change", State: model.IntentDirtyStateRemains, Reasons: []string{"authorship unproven"}}}})
	r.Assessments = append(r.Assessments, model.DerivedSessionAssessment{SessionID: "partial", Intents: []model.IntentAssessment{{IntentID: "docs", State: model.IntentUnresolved, SatisfiedCriteria: []string{"test-added"}, UnresolvedCriteria: []string{"docs"}}}})
	r.Assessments = append(r.Assessments, model.DerivedSessionAssessment{SessionID: "cancelled", Intents: []model.IntentAssessment{{IntentID: "old", State: model.IntentSuperseded, Reasons: []string{"explicit user cancellation"}}}})
	var b bytes.Buffer
	if err := Text(&b, &r, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Substantiated review 2; unverified candidates 2000; ruled down 1", "dirty / change", "Counter-evidence: observed criteria [test-added]"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(b.String(), "candidate-0 /") || strings.Contains(b.String(), "cancelled / old") {
		t.Fatal("default report flooded with background candidates")
	}
	b.Reset()
	if err := JSON(&b, &r); err != nil {
		t.Fatal(err)
	}
	var got Result
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Reconciliation.Candidates) != 2000 || len(got.Assessments) != 2003 {
		t.Fatal("raw records lost")
	}
	b.Reset()
	if err := Text(&b, &r, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "explicit user cancellation") {
		t.Fatal("rule-down evidence missing")
	}
}

func TestSessionCorrelationProgressAndTimings(t *testing.T) {
	native := model.Result{}
	var progress []string
	e := Engine{Native: &native, HostID: "fixture", Progress: func(s string) { progress = append(progress, s) }}
	r := e.Correlate(context.Background(), []model.Session{{ID: "unknown", HostID: "foreign", Intents: []model.Intent{{ID: "todo"}}}})
	if len(progress) < 2 || r.Timings["correlation_total"] <= 0 || r.Timings["association"] <= 0 {
		t.Fatal("missing session progress/timing")
	}
	if r.Assessments[0].Intents[0].State != model.IntentUnknown {
		t.Fatal("instrumentation changed certainty")
	}
}
