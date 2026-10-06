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

func eventFixture(text, kind string, at time.Time) model.IntentEvent {
	sum := sha256.Sum256([]byte(text))
	return model.IntentEvent{Kind: kind, At: at, Match: "exact_payload", DescriptionSHA256: hex.EncodeToString(sum[:]), Provenance: model.ContextProvenance{Provider: "synthetic", Artifact: "synthetic-events.json", RecordID: "event", ObservedAt: at.Add(time.Minute), Evidence: []string{"checked-exact-source-payload"}}}
}
func TestControlBoilerplateIsNotLaterHumanActivity(t *testing.T) {
	r := unknownFixture()
	r.Sessions = r.Sessions[:3]
	for n := range r.Sessions {
		r.Sessions[n].ProviderSessionID = r.Sessions[n].ID
		r.Sessions[n].Intents[0].Description = abortedControl
	}
	before, _ := json.Marshal(r.Assessments)
	q := FindUnknowns(context.Background(), &r)
	for _, g := range q.Groups {
		if len(g.Suggestions) != 0 || g.ObservationKind != "control_boilerplate_candidate" {
			t.Fatal("control boilerplate promoted to activity", g)
		}
	}
	i := &r.Sessions[0].Intents[0]
	i.Origin = "harness_control"
	e := eventFixture(i.Description, "harness_control", *r.Sessions[0].LastObservedAt)
	i.SourceEvent = &e
	q = FindUnknowns(context.Background(), &r)
	found := false
	for _, g := range q.Groups {
		if g.ObservationKind == "harness_control_event" {
			found = true
		}
	}
	if !found {
		t.Fatal("typed event provenance lost")
	}
	after, _ := json.Marshal(r.Assessments)
	if string(before) != string(after) {
		t.Fatal("interruption cancelled prior work")
	}
	for _, description := range []string{"What does <turn_aborted> mean?", "Explain this sample: " + abortedControl, abortedControl + "\nPlease fix parser.go"} {
		i.Description = description
		if controlOnly(i) || typedControl(i) {
			t.Fatal("real request suppressed")
		}
	}
}
func revertFixture() Result {
	r := contextFixture()
	s := &r.Sessions[0]
	i := &s.Intents[0]
	i.Origin = "residual_dirty_observation"
	i.Description = "Reverted this thread’s changes. I left the remaining GUIDE.md diff alone because it appears pre-existing/unrelated to this thread."
	e := eventFixture(i.Description, "assistant_message", *i.RecordedAt)
	i.SourceEvent = &e
	// A subsequent instruction is supplied; the assistant report alone is never cancellation evidence.
	text := "Revert the changes from this thread and archive"
	request := model.ScopedRevert{ID: "revert", Target: model.IntentRef{SessionID: s.ID, IntentID: i.ID}, HostID: s.HostID, ThreadID: s.ProviderSessionID, Actor: "user", Request: text, Event: eventFixture(text, "user_message", i.RecordedAt.Add(-time.Hour))}
	s.Context = &model.ContextEvidence{SchemaVersion: 1, ScopedReverts: []model.ScopedRevert{request}}
	return r
}
func TestScopedRevertRetainsResidualRiskAndLifecycle(t *testing.T) {
	r := revertFixture()
	before, _ := json.Marshal(r.Assessments)
	q := FindUnknowns(context.Background(), &r)
	g := groupFor(q, "board")
	if len(g.Matches) != 1 || g.Matches[0].Kind != "scoped_user_revert_request" || g.ObservationKind != "residual_dirty_observation" || len(g.Suggestions) != 0 {
		t.Fatal(g)
	}
	after, _ := json.Marshal(r.Assessments)
	if string(before) != string(after) {
		t.Fatal("revert request became cancellation/completion")
	}
	r.Sessions[0].Context = nil
	q = FindUnknowns(context.Background(), &r)
	if len(groupFor(q, "board").Matches) != 0 {
		t.Fatal("assistant claim alone treated as instruction")
	}
}
func TestScopedRevertNegativeBindings(t *testing.T) {
	cases := map[string]func(*model.ScopedRevert){
		"negation": func(w *model.ScopedRevert) {
			w.Request = "Do not revert the changes from this thread and archive"
			w.Event = eventFixture(w.Request, "user_message", w.Event.At)
		},
		"quoted": func(w *model.ScopedRevert) {
			w.Request = "Example: 'Revert the changes from this thread and archive'"
			w.Event = eventFixture(w.Request, "user_message", w.Event.At)
		},
		"hypothetical": func(w *model.ScopedRevert) {
			w.Request = "If needed, revert the changes from this thread and archive"
			w.Event = eventFixture(w.Request, "user_message", w.Event.At)
		},
		"assistant": func(w *model.ScopedRevert) { w.Actor = "assistant" },
		"thread":    func(w *model.ScopedRevert) { w.ThreadID = "different-thread" },
		"host":      func(w *model.ScopedRevert) { w.HostID = "other-host" },
		"target":    func(w *model.ScopedRevert) { w.Target.IntentID = "merge-a" },
		"wrong-chronology": func(w *model.ScopedRevert) {
			w.Event.At = w.Event.At.Add(3 * time.Hour)
			w.Event.Provenance.ObservedAt = w.Event.At.Add(time.Minute)
		},
		"missing-provenance": func(w *model.ScopedRevert) { w.Event.Provenance.Evidence = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := revertFixture()
			change(&r.Sessions[0].Context.ScopedReverts[0])
			q := FindUnknowns(context.Background(), &r)
			if len(groupFor(q, "board").Matches) != 0 {
				t.Fatal("unrelated/unsupported request matched")
			}
		})
	}
}
func stashFixture() Result {
	r := contextFixture()
	s := &r.Sessions[0]
	i := &s.Intents[0]
	i.Artifacts = []model.RequestedArtifact{{Path: "scripts/review.py"}, {Path: "docs/triage.md"}, {Path: "scripts/apply.ps1"}}
	s.Associations = []model.SessionAssociation{{HostID: s.HostID, RepoID: "repo-A", CommonGitDir: "/synthetic/repo-A/.git"}}
	s.Context = &model.ContextEvidence{SchemaVersion: 1}
	for n, p := range []string{"scripts/review.py", "docs/triage.md", "scripts/plan.py"} {
		s.Context.StashArtifacts = append(s.Context.StashArtifacts, model.StashArtifact{ID: p, Target: model.IntentRef{SessionID: s.ID, IntentID: i.ID}, HostID: s.HostID, RepoID: "repo-A", CommonGitDir: "/synthetic/repo-A/.git", StashOID: strings.Repeat("a", 40), ParentOID: strings.Repeat("b", 40), ParentNumber: 3, Path: p, BlobOID: strings.Repeat(string(rune('c'+n)), 40), ObjectFormat: "sha1", ObjectType: "blob", CheckedAt: i.RecordedAt.Add(time.Hour), Provenance: eventFixture("checked-stash", "tool_result", i.RecordedAt.Add(time.Hour)).Provenance})
	}
	return r
}
func TestStashSourceAvailabilityIsPerArtifactAndNotCompletion(t *testing.T) {
	r := stashFixture()
	before, _ := json.Marshal(r)
	q := FindUnknowns(context.Background(), &r)
	g := groupFor(q, "board")
	if len(g.Matches) != 2 {
		t.Fatal("expected only two requested artifacts", g.Matches)
	}
	for _, m := range g.Matches {
		if m.Kind != "SOURCE_AVAILABLE_IN_STASH" || m.StashArtifact.ParentNumber != 3 {
			t.Fatal(m)
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("availability changed native/lifecycle observations")
	}
	if !strings.Contains(strings.Join(g.Gaps, " "), "later script version") {
		t.Fatal("version limit hidden")
	}
}
func TestStashWitnessNegativeBindings(t *testing.T) {
	cases := map[string]func(*Result){
		"other-repo":       func(r *Result) { r.Sessions[0].Context.StashArtifacts[0].RepoID = "repo-B" },
		"other-common-dir": func(r *Result) { r.Sessions[0].Context.StashArtifacts[0].CommonGitDir = "/synthetic/repo-B/.git" },
		"tracked-parent":   func(r *Result) { r.Sessions[0].Context.StashArtifacts[0].ParentNumber = 2 },
		"missing-blob":     func(r *Result) { r.Sessions[0].Context.StashArtifacts[0].BlobOID = "" },
		"wrong-content":    func(r *Result) { r.Sessions[0].Intents[0].Artifacts[0].ExpectedBlobOID = strings.Repeat("f", 40) },
		"basename-only":    func(r *Result) { r.Sessions[0].Context.StashArtifacts[0].Path = "elsewhere/review.py" },
		"missing-target":   func(r *Result) { r.Sessions[0].Intents[0].TargetResolution = nil },
		"missing-event":    func(r *Result) { r.Sessions[0].Intents[0].SourceEvent = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := stashFixture()
			r.Sessions[0].Context.StashArtifacts = r.Sessions[0].Context.StashArtifacts[:1]
			change(&r)
			q := FindUnknowns(context.Background(), &r)
			if len(groupFor(q, "board").Matches) != 0 {
				t.Fatal("bad stash witness accepted")
			}
		})
	}
}
func TestBatch02ContextMergeAndPrivacy(t *testing.T) {
	r := revertFixture()
	a := r.Sessions[0].Context
	b := copiedContext(t, a)
	merged, err := mergeContext(a, &b)
	if err != nil || len(merged.ScopedReverts) != 1 {
		t.Fatal("repeat failed", err)
	}
	b.ScopedReverts[0].ThreadID = "other"
	if _, err = mergeContext(a, &b); err == nil {
		t.Fatal("conflicting revert accepted")
	}
	r = stashFixture()
	a = r.Sessions[0].Context
	b = copiedContext(t, a)
	merged, err = mergeContext(a, &b)
	if err != nil || len(merged.StashArtifacts) != 3 {
		t.Fatal("stash merge failed", err)
	}
	b.StashArtifacts[0].BlobOID = strings.Repeat("f", 40)
	if _, err = mergeContext(a, &b); err == nil {
		t.Fatal("conflicting stash accepted")
	}
	r = revertFixture()
	q := FindUnknowns(context.Background(), &r)
	r.UnknownQueue = &q
	before := append([]model.DerivedSessionAssessment{}, r.Assessments...)
	Sanitize(&r, true)
	if r.Sessions[0].Context.ScopedReverts[0].Request != "" {
		t.Fatal("retained request text")
	}
	if !reflect.DeepEqual(before, r.Assessments) {
		t.Fatal("privacy changed states")
	}
}

func TestControlNegativesRemainFuzzyEligible(t *testing.T) {
	for _, text := range []string{"Explain the interruption semantics of <turn_aborted>", "Please explain this sample: " + abortedControl, abortedControl + "\nFix parser separator traversal regression"} {
		r := unknownFixture()
		r.Sessions = r.Sessions[:2]
		r.Sessions[0].Intents[0].Description = text
		r.Sessions[1].Intents[0].Description = text
		r.Sessions[1].ProviderSessionID = "independent-thread"
		r.Sessions[1].LastObservedAt = func() *time.Time { v := r.Sessions[0].LastObservedAt.Add(time.Hour); return &v }()
		q := FindUnknowns(context.Background(), &r)
		count := 0
		for _, g := range q.Groups {
			count += len(g.Suggestions)
		}
		if count != 1 {
			t.Fatal("substantive/quoted user requests lost fuzzy eligibility", text, count)
		}
	}
}
func TestResidualTypingRequiresActualPayloadBinding(t *testing.T) {
	r := revertFixture()
	i := &r.Sessions[0].Intents[0]
	if !residualObservation(i) {
		t.Fatal("positive missing")
	}
	i.SourceEvent.DescriptionSHA256 = strings.Repeat("0", 64)
	if residualObservation(i) {
		t.Fatal("unbound producer label accepted")
	}
}

func TestBatch02NormalizedSourcesAreReadOnly(t *testing.T) {
	for _, r := range []Result{revertFixture(), stashFixture()} {
		for n := range r.Sessions[0].Intents {
			r.Sessions[0].Intents[n].SessionID = r.Sessions[0].ID
		}
		data, err := json.Marshal(NormalizedArtifact{SchemaVersion: 1, Sessions: r.Sessions})
		if err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(t.TempDir(), "synthetic.json")
		if err = os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
		source := FileSource{Paths: []string{filename}, Format: "normalized"}
		artifacts, err := source.Discover()
		if err != nil {
			t.Fatal(err)
		}
		imported, err := source.ReadSessions(context.Background(), artifacts[0])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(imported[0].Context, r.Sessions[0].Context) {
			t.Fatal("context contract lost during import")
		}
		after, err := os.ReadFile(filename)
		if err != nil || string(after) != string(data) {
			t.Fatal("source changed")
		}
	}
}

func TestExtractedFragmentCannotClaimWholeEventBinding(t *testing.T) {
	r := revertFixture()
	i := &r.Sessions[0].Intents[0]
	full := "First completed a different step.\n" + i.Description + "\nMore context follows."
	event := eventFixture(full, "assistant_message", *i.RecordedAt)
	i.SourceEvent = &event
	q := FindUnknowns(context.Background(), &r)
	g := groupFor(q, "board")
	if len(g.Matches) != 0 || g.ObservationKind == "residual_dirty_observation" {
		t.Fatal("fragment pretended to be full source event")
	}
	if !strings.Contains(strings.Join(g.Gaps, " "), "exact description hash") {
		t.Fatal("fragment provenance gap hidden")
	}
}

func TestNoArtifactMetadataKeepsBatch01GroupIdentity(t *testing.T) {
	r := contextFixture()
	s := &r.Sessions[0]
	i := &s.Intents[0]
	payload, _ := json.Marshal(struct {
		Description, Kind, Origin, TargetRepoID string
		Explicit                                bool
		Files, Symbols, Commands                []string
		Completion                              []model.CriterionGroup
		Tracker                                 *model.TrackerSubject
	}{strings.Join(strings.Fields(i.Description), " "), i.Kind, i.Origin, i.TargetRepoID, i.Explicit, i.Files, i.Symbols, i.Commands, i.Completion, i.Tracker})
	expected := model.ID("unknown-", s.HostID, s.Harness, threadIdentity(s), scope(s), string(payload))
	if unknownGroupKey(s, i) != expected {
		t.Fatal("unrelated group identity changed")
	}
}
