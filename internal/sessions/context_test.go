package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MTG-Thomas/spuro/internal/model"
)

func contextFixture() Result {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	intents := []model.Intent{
		{ID: "board", Description: "Style pages, dependencies, tests, and notes", Origin: "task_board", RecordedAt: &at, TargetRepoID: "repo-A", Evidence: []string{"board-row"}},
		{ID: "merge-a", Description: "Cannot merge PR42", RecordedAt: &at, Tracker: &model.TrackerSubject{ForgeHost: "github.com", Repository: "example/repo-A", Kind: "pull_request", Number: 42, Action: "merge"}, Evidence: []string{"merge-a-row"}},
		{ID: "merge-b", Description: "Cannot merge PR42", RecordedAt: &at, Tracker: &model.TrackerSubject{ForgeHost: "github.com", Repository: "example/repo-B", Kind: "pull_request", Number: 42, Action: "merge"}, Evidence: []string{"merge-b-row"}},
		{ID: "protection", Description: "Review branch protection", RecordedAt: &at, Evidence: []string{"protection-row"}},
	}
	for n := range intents {
		i := &intents[n]
		sum := sha256.Sum256([]byte(i.Description))
		kind := "assistant_message"
		if i.Origin == "task_board" {
			kind = "todo_write"
		}
		i.SourceEvent = &model.IntentEvent{Kind: kind, At: at, Match: "exact_payload", DescriptionSHA256: hex.EncodeToString(sum[:]), Provenance: model.ContextProvenance{Provider: "fixture", Artifact: "synthetic-events.json", RecordID: i.ID + "-event", ObservedAt: at.Add(time.Minute), Evidence: []string{i.ID + "-event"}}}
	}
	intents[0].TargetResolution = &model.TargetResolution{HostID: "host", RepoID: "repo-A", Method: "verified_user_path", Provenance: model.ContextProvenance{Provider: "fixture", Artifact: "synthetic-target-resolution.json", RecordID: "target-map", ObservedAt: at.Add(time.Minute), Evidence: []string{"user-path-record", "checked-checkout-map"}}}
	r := Result{Format: "spuro-sessions", SchemaVersion: 1, HostID: "host", Sessions: []model.Session{{ID: "thread", HostID: "host", ProviderSessionID: "provider-thread", Intents: intents}}}
	assessment := model.DerivedSessionAssessment{SessionID: "thread", Summary: model.SessionUnknown}
	for _, i := range intents {
		assessment.Intents = append(assessment.Intents, model.IntentAssessment{IntentID: i.ID, State: model.IntentUnknown})
	}
	r.Assessments = []model.DerivedSessionAssessment{assessment}
	later := at.Add(time.Hour)
	provenance := model.ContextProvenance{Provider: "operator-reviewed-fixture", Artifact: "synthetic-observations.json", RecordID: "request-row", ObservedAt: later.Add(time.Minute), Evidence: []string{"request-row"}}
	request := model.BoundedRequest{ID: "scope", Target: model.IntentRef{SessionID: "thread", IntentID: "board"}, Actor: "user", At: later, HostID: "host", RepoID: "repo-A", AllowedPaths: []string{"src/components/panel.tsx"}, OtherWorkForbidden: true, Provenance: provenance}
	provenance.RecordID = "tracker-row"
	provenance.Evidence = []string{"tracker-row"}
	tracker := model.TrackerWitness{ID: "merged", Target: model.IntentRef{SessionID: "thread", IntentID: "merge-a"}, Subject: *intents[1].Tracker, Outcome: "merged", EventAt: later, CheckedBy: "fixture-checker", Provenance: provenance}
	r.Sessions[0].Context = &model.ContextEvidence{SchemaVersion: 1, BoundedRequests: []model.BoundedRequest{request}, TrackerWitnesses: []model.TrackerWitness{tracker}}
	return r
}
func groupFor(q UnknownQueue, id string) *UnknownGroup {
	for n := range q.Groups {
		for _, m := range q.Groups[n].Members {
			if m.IntentID == id {
				return &q.Groups[n]
			}
		}
	}
	return nil
}
func TestBoundedRequestIsScopeEvidenceNotBoardCompletion(t *testing.T) {
	r := contextFixture()
	before, _ := json.Marshal(r.Assessments)
	q := FindUnknowns(context.Background(), &r)
	g := groupFor(q, "board")
	if g == nil || len(g.Matches) != 1 || g.Matches[0].Kind != "bounded_user_request_record" || !reflect.DeepEqual(g.Matches[0].AllowedPaths, []string{"src/components/panel.tsx"}) {
		t.Fatal("bounded scope witness missing")
	}
	if !strings.Contains(strings.Join(g.Gaps, " "), "broad task board") || !strings.Contains(g.NextStep, "do not resume the whole task board") {
		t.Fatal("scope mismatch suppressed")
	}
	after, _ := json.Marshal(r.Assessments)
	if string(after) != string(before) {
		t.Fatal("board classified as completed/superseded")
	}
	r.Sessions[0].Context.BoundedRequests[0].Actor = "assistant"
	if ValidateContext(r.Sessions[0].Context) == nil {
		t.Fatal("assistant plan became user authorization")
	}
}
func TestTrackerWitnessIsExactPerIntentAndAdvisory(t *testing.T) {
	r := contextFixture()
	q := FindUnknowns(context.Background(), &r)
	a, b, protection := groupFor(q, "merge-a"), groupFor(q, "merge-b"), groupFor(q, "protection")
	if len(a.Matches) != 1 || a.Matches[0].Kind != "supplied_checked_tracker_merge" || a.Matches[0].Provenance.RecordID != "tracker-row" {
		t.Fatal("narrow tracker witness missing")
	}
	if len(b.Matches) != 0 || len(protection.Matches) != 0 {
		t.Fatal("PR number leaked across repositories/intents")
	}
	if r.Assessments[0].Summary != model.SessionUnknown {
		t.Fatal("whole thread completed")
	}
	witness := &r.Sessions[0].Context.TrackerWitnesses[0]
	witness.Target.IntentID = "merge-b"
	q = FindUnknowns(context.Background(), &r)
	if len(groupFor(q, "merge-b").Matches) != 0 {
		t.Fatal("different repo PR42 matched")
	}
	witness.Target.IntentID = "merge-a"
	witness.Outcome = "closed"
	q = FindUnknowns(context.Background(), &r)
	if len(groupFor(q, "merge-a").Matches) != 0 {
		t.Fatal("closure treated as merge")
	}
	witness.Outcome = "merged"
	witness.Subject.ForgeHost = "different.example"
	q = FindUnknowns(context.Background(), &r)
	if len(groupFor(q, "merge-a").Matches) != 0 {
		t.Fatal("different forge matched")
	}
}
func TestContextMissingChronologyAndTargetDoNotBorrowCWD(t *testing.T) {
	r := contextFixture()
	r.Sessions[0].Intents[0].TargetRepoID = ""
	r.Sessions[0].Associations = []model.SessionAssociation{{HostID: "host", RepoID: "repo-A"}}
	r.Sessions[0].Intents[1].RecordedAt = nil
	q := FindUnknowns(context.Background(), &r)
	if len(groupFor(q, "board").Matches) != 0 || len(groupFor(q, "merge-a").Matches) != 0 {
		t.Fatal("missing target/event chronology invented")
	}
	r = contextFixture()
	r.Sessions[0].Context.TrackerWitnesses[0].EventAt = r.Sessions[0].Intents[1].RecordedAt.Add(-time.Hour)
	q = FindUnknowns(context.Background(), &r)
	if len(groupFor(q, "merge-a").Matches) != 0 {
		t.Fatal("earlier tracker event treated as later resolution")
	}
}
func TestContextValidationAndConflictingMerge(t *testing.T) {
	r := contextFixture()
	c := r.Sessions[0].Context
	if err := ValidateContext(c); err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(c)
	var copy model.ContextEvidence
	if err := json.Unmarshal(original, &copy); err != nil {
		t.Fatal(err)
	}
	merged, err := mergeContext(c, &copy)
	if err != nil || len(merged.BoundedRequests) != 1 || len(merged.TrackerWitnesses) != 1 {
		t.Fatal("identical repeated observation not merged")
	}
	copy.BoundedRequests[0].RepoID = "other"
	if _, err := mergeContext(c, &copy); err == nil {
		t.Fatal("contradictory observation silently replaced")
	}
	copy = copiedContext(t, c)
	copy.SchemaVersion = 2
	if ValidateContext(&copy) == nil {
		t.Fatal("unknown context schema accepted")
	}
	copy = copiedContext(t, c)
	copy.BoundedRequests[0].AllowedPaths = []string{"../escape"}
	if ValidateContext(&copy) == nil {
		t.Fatal("unbounded paths accepted")
	}
	copy = copiedContext(t, c)
	copy.TrackerWitnesses[0].Provenance.RecordID = ""
	if ValidateContext(&copy) == nil {
		t.Fatal("missing provenance accepted")
	}
}
func copiedContext(t *testing.T, c *model.ContextEvidence) model.ContextEvidence {
	t.Helper()
	data, _ := json.Marshal(c)
	var out model.ContextEvidence
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNormalizedSourceRetainsContextAndRejectsUnknownContract(t *testing.T) {
	r := contextFixture()
	for n := range r.Sessions[0].Intents {
		r.Sessions[0].Intents[n].SessionID = "thread"
	}
	data, _ := json.Marshal(NormalizedArtifact{SchemaVersion: 1, Sessions: r.Sessions})
	file := filepath.Join(t.TempDir(), "normalized.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	source := FileSource{Paths: []string{file}, Format: "normalized"}
	artifacts, err := source.Discover()
	if err != nil {
		t.Fatal(err)
	}
	imported, err := source.ReadSessions(context.Background(), artifacts[0])
	if err != nil {
		t.Fatal(err)
	}
	if imported[0].Context == nil || len(imported[0].Context.BoundedRequests) != 1 || len(imported[0].Context.TrackerWitnesses) != 1 {
		t.Fatal("source dropped typed evidence")
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(data) {
		t.Fatal("source rewritten")
	}
	r.Sessions[0].Context.SchemaVersion = 2
	data, _ = json.Marshal(NormalizedArtifact{SchemaVersion: 1, Sessions: r.Sessions})
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	artifacts, err = source.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ReadSessions(context.Background(), artifacts[0]); err == nil {
		t.Fatal("unknown context schema accepted")
	}
}

func TestNonexclusiveRequestDoesNotSupersedeBoardAndTextFreeProvenance(t *testing.T) {
	r := contextFixture()
	request := &r.Sessions[0].Context.BoundedRequests[0]
	request.OtherWorkForbidden = false
	request.Provenance.Artifact = "https://example.invalid/export?token=secretvalue"
	q := FindUnknowns(context.Background(), &r)
	g := groupFor(q, "board")
	if len(g.Matches) != 1 || strings.Contains(strings.Join(g.Gaps, " "), "broad task board") || strings.Contains(g.NextStep, "do not resume the whole task board") {
		t.Fatal("nonexclusive request became whole-board restriction")
	}
	r.UnknownQueue = &q
	Sanitize(&r, true)
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "secretvalue") || strings.Contains(string(data), "Style pages, dependencies") {
		t.Fatal("provenance/text-free redaction failed")
	}
}

func TestTodoEventCannotBorrowLastAssistantTimestamp(t *testing.T) {
	for _, change := range []string{"missing_event", "wrong_kind", "wrong_payload", "wrong_time", "cwd_resolution"} {
		t.Run(change, func(t *testing.T) {
			r := contextFixture()
			i := &r.Sessions[0].Intents[0]
			switch change {
			case "missing_event":
				i.SourceEvent = nil
			case "wrong_kind":
				i.SourceEvent.Kind = "assistant_message"
			case "wrong_payload":
				i.SourceEvent.DescriptionSHA256 = strings.Repeat("a", 64)
			case "wrong_time":
				i.SourceEvent.At = i.SourceEvent.At.Add(time.Minute)
			case "cwd_resolution":
				i.TargetResolution.Method = "cwd"
			}
			q := FindUnknowns(context.Background(), &r)
			if len(groupFor(q, "board").Matches) != 0 {
				t.Fatal("unverified event/path resolution became context evidence")
			}
		})
	}
}
