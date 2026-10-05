package model

import (
	"encoding/json"
	"testing"
)

func TestSessionAssessmentRetainsIndependentIntentStates(t *testing.T) {
	c := UnknownCoverage()
	c.GitHistory = CoverageObservation{Level: CoveragePartial, Scope: "scanned local refs", Reasons: []string{"shallow history"}}
	original := DerivedSessionAssessment{SessionID: "fixture", Summary: SessionPartiallyCompleted, Coverage: c,
		Intents: []IntentAssessment{
			{IntentID: "parser", State: IntentCompleted, Confidence: High, Coverage: c, Evidence: []string{"commit-parser"}},
			{IntentID: "test", State: IntentCompleted, Confidence: High, Coverage: c, Evidence: []string{"commit-regression"}},
			{IntentID: "docs", State: IntentFailedUnresolved, Confidence: High, Coverage: c, Reasons: []string{"later coverage partial"}},
		}}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var got DerivedSessionAssessment
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Intents) != 3 || got.Intents[2].State != IntentFailedUnresolved || got.Intents[2].Confidence != High || got.Intents[2].Coverage.GitHistory.Level != CoveragePartial {
		t.Fatalf("lost independent evidence: %+v", got)
	}
	if got.Coverage.SessionHistory.Level != CoverageUnknown {
		t.Fatal("unknown coverage lost")
	}
}

func TestUnobservedCommandExitDoesNotBecomeSuccess(t *testing.T) {
	for _, code := range []*int{nil, new(int)} {
		original := SessionCommand{ID: "fixture", ExitCode: code}
		b, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var got SessionCommand
		if err = json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if (got.ExitCode == nil) != (code == nil) {
			t.Fatal("unobserved and zero exit conflated")
		}
	}
}
