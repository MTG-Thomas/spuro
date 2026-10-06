package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MTG-Thomas/spuro/internal/model"
)

func digest(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func inheritanceFixture() Result {
	r := unknownFixture()
	r.Sessions = r.Sessions[:3]
	text := "Current blocker: service field update requires inspecting request shape"
	full := "Summary prefix.\n" + text + "\nSummary suffix."
	receipt := model.OriginReceipt{ArtifactSHA256: strings.Repeat("a", 64), Provenance: eventFixture("synthetic", "tool_result", *r.Sessions[0].LastObservedAt).Provenance}
	c := &model.InheritanceEvidence{SchemaVersion: 1}
	identity := func(thread string) model.OriginIdentity {
		return model.OriginIdentity{HostID: "host", Harness: "fixture", Provider: "synthetic", ThreadID: thread}
	}
	c.Events = []model.OriginEvent{{ID: "parent-event", OriginIdentity: identity("parent"), TurnID: "parent-turn-1", Kind: "task_complete", PayloadSHA256: digest(full), PayloadBytes: len([]byte(full)), OriginReceipt: receipt}}
	for n := range r.Sessions {
		s := &r.Sessions[n]
		s.Provider = "synthetic"
		s.ProviderSessionID = []string{"parent", "child-a", "child-b"}[n]
		s.Intents[0].Description = text
		at := r.Sessions[0].LastObservedAt.Add(time.Duration(n) * time.Hour)
		s.LastObservedAt = &at
		r.Assessments[n].Intents[0].State = model.IntentUnknown
		id := s.ProviderSessionID
		if n > 0 {
			c.Forks = append(c.Forks, model.SessionFork{ID: "fork-" + id, OriginIdentity: identity(id), ParentThreadID: "parent", OriginReceipt: receipt})
		}
		c.Occurrences = append(c.Occurrences, model.OriginOccurrence{ID: "occ-" + id, OriginIdentity: identity(id), EventID: "parent-event", TurnID: "parent-turn-1", Kind: "task_complete", PayloadSHA256: digest(full), PayloadBytes: len([]byte(full)), Inherited: n > 0, OriginReceipt: receipt})
		start := len([]byte("Summary prefix.\n"))
		c.Fragments = append(c.Fragments, model.OriginFragment{ID: "frag-" + id, Target: model.IntentRef{SessionID: s.ID, IntentID: "intent"}, OccurrenceID: "occ-" + id, Match: "exact_fragment", UTF8BoundariesVerified: true, FragmentSHA256: digest(text), ByteStart: start, ByteEnd: start + len([]byte(text)), OriginReceipt: receipt})
	}
	r.Sessions[0].Inheritance = c
	return r
}
func fuzzyTotal(q UnknownQueue) int {
	n := 0
	for _, g := range q.Groups {
		n += len(g.Suggestions)
	}
	return n
}
func TestInheritedOriginSuppressesOnlySameFragmentActivity(t *testing.T) {
	r := inheritanceFixture()
	before, _ := json.Marshal(r)
	baseline := r
	baseline.Sessions = append([]model.Session{}, r.Sessions...)
	baseline.Sessions[0].Inheritance = nil
	old := FindUnknowns(context.Background(), &baseline)
	if fuzzyTotal(old) == 0 {
		t.Fatal("fixture did not reproduce inherited activity")
	}
	q := FindUnknowns(context.Background(), &r)
	if fuzzyTotal(q) != 0 || len(q.Groups) != 3 || q.Sightings != 3 {
		t.Fatal("suppression lost sightings", q)
	}
	for _, g := range q.Groups {
		found := false
		for _, ref := range g.IdentityCandidates {
			if ref.Kind == "same_inherited_source_fragment" {
				found = true
			}
		}
		if !found {
			t.Fatal("inherited evidence not retained")
		}
		for _, ref := range g.IdentityCandidates {
			if ref.SessionID == g.Members[0].SessionID {
				t.Fatal("inherited self-reference")
			}
		}
	}
	ids := map[string]bool{}
	for _, g := range old.Groups {
		ids[g.ID] = true
	}
	for _, g := range q.Groups {
		if !ids[g.ID] {
			t.Fatal("group ID changed")
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("snapshot modified")
	}
	// A wholly new child request is not inherited, even with same prose.
	r.Sessions[0].Inheritance.Occurrences[2].Inherited = false
	if fuzzyTotal(FindUnknowns(context.Background(), &r)) == 0 {
		t.Fatal("new child request suppressed")
	}
}
func TestInheritedOriginFailsClosed(t *testing.T) {
	cases := map[string]func(*Result){
		"missing-parent-session": func(r *Result) { r.Sessions[0].ProviderSessionID = "missing-original-parent" },
		"missing-parent-event":   func(r *Result) { r.Sessions[0].Inheritance.Events = nil },
		"distinct-turn":          func(r *Result) { r.Sessions[0].Inheritance.Occurrences[1].TurnID = "child-new-turn" },
		"modified-payload":       func(r *Result) { r.Sessions[0].Inheritance.Occurrences[1].PayloadSHA256 = strings.Repeat("b", 64) },
		"missing-ancestry":       func(r *Result) { r.Sessions[0].Inheritance.Forks = nil },
		"wrong-host":             func(r *Result) { r.Sessions[0].Inheritance.Occurrences[1].HostID = "other" },
		"wrong-harness":          func(r *Result) { r.Sessions[0].Inheritance.Occurrences[1].Harness = "different" },
		"wrong-provider":         func(r *Result) { r.Sessions[0].Inheritance.Occurrences[1].Provider = "different" },
		"cycle": func(r *Result) {
			r.Sessions[0].Inheritance.Forks[0].ParentThreadID = "child-b"
			r.Sessions[0].Inheritance.Forks[1].ParentThreadID = "child-a"
		},
		"missing-thread":     func(r *Result) { r.Sessions[0].Inheritance.Forks[0].ParentThreadID = "missing" },
		"fragment-hash":      func(r *Result) { r.Sessions[0].Inheritance.Fragments[1].FragmentSHA256 = strings.Repeat("c", 64) },
		"fragment-span":      func(r *Result) { r.Sessions[0].Inheritance.Fragments[1].ByteStart++ },
		"out-of-range":       func(r *Result) { f := &r.Sessions[0].Inheritance.Fragments[1]; f.ByteStart += 1000; f.ByteEnd += 1000 },
		"missing-provenance": func(r *Result) { r.Sessions[0].Inheritance.Fragments[1].Provenance.Evidence = nil },
		"fragment-not-whole": func(r *Result) { r.Sessions[0].Inheritance.Fragments[1].Match = "exact_payload" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := inheritanceFixture()
			change(&r)
			q := FindUnknowns(context.Background(), &r)
			if fuzzyTotal(q) == 0 {
				t.Fatal("unproven inheritance suppressed activity")
			}
		})
	}
}
func TestInheritedOriginUTF8AndDifferentFragment(t *testing.T) {
	r := inheritanceFixture()
	c := r.Sessions[0].Inheritance
	r.Sessions[1].Intents[0].Description = "Current blocker: service field update requires inspecting request shapé"
	f := &c.Fragments[1]
	f.FragmentSHA256 = digest(r.Sessions[1].Intents[0].Description)
	f.ByteEnd = f.ByteStart + len([]byte(r.Sessions[1].Intents[0].Description))
	// This producer-supplied alternate fragment cannot identify the old exact fragment.
	if fuzzyTotal(FindUnknowns(context.Background(), &r)) == 0 {
		t.Fatal("different UTF8 fragment suppressed")
	}
}
func TestInheritanceSchemaAndConflicts(t *testing.T) {
	r := inheritanceFixture()
	a := r.Sessions[0].Inheritance
	data, _ := json.Marshal(a)
	var b model.InheritanceEvidence
	json.Unmarshal(data, &b)
	merged, e := mergeInheritance(a, &b)
	if e != nil || len(merged.Events) != 1 || len(merged.Fragments) != 3 {
		t.Fatal("identical records not deduped", e)
	}
	b.Events[0].PayloadBytes++
	if _, e = mergeInheritance(a, &b); e == nil {
		t.Fatal("conflicting source event accepted")
	}
	b.SchemaVersion = 2
	if ValidateInheritance(&b) == nil {
		t.Fatal("unknown schema accepted")
	}
}

func TestInheritanceSourceReadOnlyAndAmbiguity(t *testing.T) {
	r := inheritanceFixture()
	for n := range r.Sessions {
		r.Sessions[n].Intents[0].SessionID = r.Sessions[n].ID
	}
	data, e := json.Marshal(NormalizedArtifact{SchemaVersion: 1, Sessions: r.Sessions})
	if e != nil {
		t.Fatal(e)
	}
	filename := filepath.Join(t.TempDir(), "synthetic-inheritance.json")
	if e = os.WriteFile(filename, data, 0600); e != nil {
		t.Fatal(e)
	}
	source := FileSource{Paths: []string{filename}, Format: "normalized"}
	artifacts, e := source.Discover()
	if e != nil {
		t.Fatal(e)
	}
	imported, e := source.ReadSessions(context.Background(), artifacts[0])
	if e != nil || len(imported[0].Inheritance.Fragments) != 3 {
		t.Fatal("ingestion failed", e)
	}
	after, e := os.ReadFile(filename)
	if e != nil || string(after) != string(data) {
		t.Fatal("source modified")
	}
	// A second conflicting record with the same ID cannot be selected by input order.
	r.Sessions[1].Inheritance = &model.InheritanceEvidence{SchemaVersion: 1, Events: append([]model.OriginEvent{}, r.Sessions[0].Inheritance.Events...)}
	r.Sessions[1].Inheritance.Events[0].PayloadBytes++
	if fuzzyTotal(FindUnknowns(context.Background(), &r)) == 0 {
		t.Fatal("conflicting source selected")
	}
}
func TestMultipleFragmentOriginsRemainAmbiguous(t *testing.T) {
	r := inheritanceFixture()
	c := r.Sessions[0].Inheritance
	e := c.Events[0]
	e.ID = "second-event"
	e.TurnID = "second-turn"
	c.Events = append(c.Events, e)
	o := c.Occurrences[0]
	o.ID = "second-occurrence"
	o.EventID = e.ID
	o.TurnID = e.TurnID
	c.Occurrences = append(c.Occurrences, o)
	f := c.Fragments[0]
	f.ID = "second-fragment"
	f.OccurrenceID = o.ID
	c.Fragments = append(c.Fragments, f)
	f.ID = "third-fragment"
	f.OccurrenceID = c.Occurrences[0].ID
	c.Fragments = append(c.Fragments, f)
	x := indexOrigins(r.Sessions)
	if _, ok := x.bindings[intentRefKey(f.Target)]; ok {
		t.Fatal("ambiguous origin restored by later repeated record")
	}
}

func TestConflictingForkParentsDoNotSuppress(t *testing.T) {
	r := inheritanceFixture()
	c := r.Sessions[0].Inheritance
	edge := c.Forks[0]
	edge.ID = "conflicting-edge"
	edge.ParentThreadID = "child-b"
	c.Forks = append(c.Forks, edge)
	if fuzzyTotal(FindUnknowns(context.Background(), &r)) == 0 {
		t.Fatal("ambiguous fork selected")
	}
}

func TestInheritedFragmentUsesUTF8ByteOffsets(t *testing.T) {
	r := inheritanceFixture()
	c := r.Sessions[0].Inheritance
	prefix := "Résumé 🎯\n"
	text := "Current blocker: service field update requires inspecting café request shape"
	full := prefix + text + "\nEnd"
	c.Events[0].PayloadSHA256 = digest(full)
	c.Events[0].PayloadBytes = len([]byte(full))
	for n := range r.Sessions {
		r.Sessions[n].Intents[0].Description = text
		c.Occurrences[n].PayloadSHA256 = digest(full)
		c.Occurrences[n].PayloadBytes = len([]byte(full))
		c.Fragments[n].FragmentSHA256 = digest(text)
		c.Fragments[n].ByteStart = len([]byte(prefix))
		c.Fragments[n].ByteEnd = len([]byte(prefix + text))
	}
	if fuzzyTotal(FindUnknowns(context.Background(), &r)) != 0 {
		t.Fatal("valid UTF8 byte spans not recognized")
	}
	f := &c.Fragments[1]
	start, end := f.ByteStart, f.ByteEnd
	f.ByteStart = len([]rune(prefix))
	f.ByteEnd = len([]rune(prefix + text))
	if fuzzyTotal(FindUnknowns(context.Background(), &r)) == 0 {
		t.Fatal("character offsets accepted as UTF8 bytes")
	}
	f.ByteStart = start
	f.ByteEnd = end
	f.UTF8BoundariesVerified = false
	if fuzzyTotal(FindUnknowns(context.Background(), &r)) == 0 {
		t.Fatal("unverified interior-codepoint boundaries accepted")
	}
	f.ByteStart = 1
	f.ByteEnd = 1 + len([]byte(text))
	f.UTF8BoundariesVerified = false
	if fuzzyTotal(FindUnknowns(context.Background(), &r)) == 0 {
		t.Fatal("interior UTF8 position accepted without checked boundary proof")
	}
}
