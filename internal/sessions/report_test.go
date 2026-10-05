package sessions

import (
	"bytes"
	"encoding/json"
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
