package sessions

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MTG-Thomas/spuro/internal/model"
)

func unknownFixture() Result {
	a := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := a.Add(time.Hour)
	session := func(id, thread, host, description string, at time.Time) model.Session {
		return model.Session{ID: id, ProviderSessionID: thread, HostID: host, Harness: "fixture", LastObservedAt: &at, Associations: []model.SessionAssociation{{HostID: host, RepoID: "repo"}}, Intents: []model.Intent{{ID: "intent", Description: description}}}
	}
	r := Result{Format: "spuro-sessions", SchemaVersion: 1, HostID: "host", Sessions: []model.Session{
		session("old", "thread", "host", "Next: Windows separator traversal regression coverage", a),
		session("duplicate", "thread", "host", "Next: Windows separator traversal regression coverage", a),
		session("later", "later-thread", "host", "Windows separator traversal regression coverage implemented", b),
		session("foreign", "other-thread", "foreign", "Windows separator traversal regression coverage implemented", b),
	}}
	for _, s := range r.Sessions {
		state := model.IntentUnknown
		if s.ID == "later" || s.ID == "foreign" {
			state = model.IntentCompleted
		}
		r.Assessments = append(r.Assessments, model.DerivedSessionAssessment{SessionID: s.ID, Intents: []model.IntentAssessment{{IntentID: "intent", State: state, Coverage: model.UnknownCoverage()}}})
	}
	return r
}
func TestUnknownDuplicatesFuzzyAndCoverage(t *testing.T) {
	r := unknownFixture()
	before := append([]model.DerivedSessionAssessment{}, r.Assessments...)
	q := FindUnknowns(context.Background(), &r)
	if q.Sightings != 2 || len(q.Groups) != 1 || len(q.Groups[0].Members) != 2 {
		t.Fatal("exact thread payload deduplication failed")
	}
	g := q.Groups[0]
	if g.Priority != "FOLLOW_UP_CANDIDATE" || len(g.Suggestions) == 0 {
		t.Fatal("missing useful later suggestion")
	}
	for _, m := range g.Suggestions {
		if m.SessionID != "later" {
			t.Fatal("cross-host fuzzy match")
		}
	}
	if len(g.Matches) != 0 || !reflect.DeepEqual(before, r.Assessments) {
		t.Fatal("fuzzy suggestion became lifecycle proof")
	}
	if !strings.Contains(strings.Join(g.Gaps, " "), "coverage incomplete") {
		t.Fatal("coverage hidden")
	}
}
func TestUnknownExactCancellationAndCachedWitness(t *testing.T) {
	r := unknownFixture()
	oid := strings.Repeat("a", 40)
	r.Sessions[0].Intents[0].Completion = []model.CriterionGroup{{Mode: "ALL", Criteria: []model.CompletionCriterion{{ID: "landed", Kind: "commit_reachable", ExpectedOID: oid, ExpectedObjectFormat: "sha1"}}}}
	r.Sessions[2].Decisions = []model.IntentDecision{{SessionID: "old", IntentID: "intent", Actor: "user", Decision: "cancel", Evidence: []string{"user-record"}}}
	r.Native = &model.Result{Repositories: []model.Repository{{ID: "repo", ObjectFormat: "sha1", Complete: true, Commits: []model.Commit{{OID: oid, Classes: []model.Reachability{model.DurableRef}}}}}}
	q := FindUnknowns(context.Background(), &r)
	var exact *UnknownGroup
	for n := range q.Groups {
		for _, m := range q.Groups[n].Members {
			if m.SessionID == "old" {
				exact = &q.Groups[n]
			}
		}
	}
	if exact == nil || exact.Priority != "EVIDENCE_READY" {
		t.Fatal("exact witnesses missing")
	}
	kinds := map[string]bool{}
	for _, m := range exact.Matches {
		kinds[m.Kind] = true
	}
	if !kinds["explicit_user_cancellation_record"] || !kinds["cached_commit_reachable"] {
		t.Fatal(kinds)
	}
	if r.Assessments[0].Intents[0].State != model.IntentUnknown {
		t.Fatal("snapshot verdict rewritten")
	}
	r.Native.Repositories[0].ChangedDuringScan = true
	q = FindUnknowns(context.Background(), &r)
	for _, g := range q.Groups {
		for _, m := range g.Matches {
			if strings.HasPrefix(m.Kind, "cached_") {
				t.Fatal("unstable snapshot used")
			}
		}
	}
}
func TestUnknownAmbiguousScopeOptionalAndNoText(t *testing.T) {
	r := unknownFixture()
	r.Sessions[0].Associations = append(r.Sessions[0].Associations, model.SessionAssociation{HostID: "host", RepoID: "another"})
	r.Sessions[0].Intents[0].Description = "Maybe optional task board: Windows separator traversal regression coverage"
	q := FindUnknowns(context.Background(), &r)
	r.UnknownQueue = &q
	Sanitize(&r, true)
	for _, g := range r.UnknownQueue.Groups {
		if g.Description != "" {
			t.Fatal("text-free queue leaked prose")
		}
		for _, m := range g.Members {
			if m.SessionID == "old" {
				if len(g.Suggestions) > 0 {
					t.Fatal("ambiguous repository suggested completion")
				}
				if !strings.Contains(strings.Join(g.Gaps, " "), "optional") {
					t.Fatal("optional scope gap missing")
				}
			}
		}
	}
}
func TestUnknownMissingChronologyDoesNotSuggestLaterWork(t *testing.T) {
	r := unknownFixture()
	r.Sessions[0].LastObservedAt = nil
	r.Sessions[1].LastObservedAt = nil
	q := FindUnknowns(context.Background(), &r)
	for _, g := range q.Groups {
		if len(g.Suggestions) != 0 {
			t.Fatal("invented chronology")
		}
	}
}

func BenchmarkUnknownReconciliation(b *testing.B) {
	r := Result{}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for n := 0; n < 2435; n++ {
		id := fmt.Sprintf("session-%d", n)
		at := start.Add(time.Duration(n) * time.Minute)
		r.Sessions = append(r.Sessions, model.Session{ID: id, HostID: "fixture", LastObservedAt: &at, Associations: []model.SessionAssociation{{HostID: "fixture", RepoID: fmt.Sprintf("repo-%d", n%50)}}, Intents: []model.Intent{{ID: "intent", Description: "Next: Windows separator traversal regression coverage"}}})
		r.Assessments = append(r.Assessments, model.DerivedSessionAssessment{SessionID: id, Intents: []model.IntentAssessment{{IntentID: "intent", State: model.IntentUnknown}}})
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		q := FindUnknowns(context.Background(), &r)
		if q.Sightings != 2435 {
			b.Fatal("sightings lost")
		}
	}
}
