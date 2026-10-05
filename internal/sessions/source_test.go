package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MTG-Thomas/spuro/internal/model"
)

func TestReviewedSyncVersionsReadOnlyAndStale(t *testing.T) {
	for _, version := range []string{"0.21.5", "0.21.6"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "deja-sync-fixture.jsonl")
			data := []byte("{\"harness\":\"codex\",\"session_id\":\"fixture\",\"project\":\"/fixture/project\",\"role\":\"assistant\",\"text\":\"Next: add regression coverage token=secretvalue\",\"time\":\"2026-10-02T21:14:00Z\",\"origin\":\"remote-fixture\"}\n")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			old := time.Date(2026, 10, 2, 21, 14, 0, 0, time.UTC)
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
			before, _ := os.Stat(path)
			source := FileSource{Paths: []string{path}, Format: "deja-sync", ProducerVersion: version, HostID: "local-fixture"}
			artifacts, err := source.Discover()
			if err != nil {
				t.Fatal(err)
			}
			ss, err := source.ReadSessions(context.Background(), artifacts[0])
			if err != nil {
				t.Fatal(err)
			}
			if len(ss) != 1 || ss[0].Coverage.ProviderHistory.Level != model.CoveragePartial || ss[0].EndedAt != nil || ss[0].StartedAt != nil || ss[0].FirstObservedAt == nil || ss[0].HostID != "remote-fixture" || ss[0].Sources[0].Latest == nil {
				t.Fatalf("invalid normalization: %+v", ss)
			}
			if len(ss[0].Intents) != 1 || ss[0].Intents[0].Description != "add regression coverage [REDACTED]" {
				t.Fatalf("tail extraction: %+v", ss[0].Intents)
			}
			after, _ := os.Stat(path)
			got, _ := os.ReadFile(path)
			if !before.ModTime().Equal(after.ModTime()) || sha256.Sum256(data) != sha256.Sum256(got) {
				t.Fatal("source mutated")
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			if len(entries) != 1 {
				t.Fatal("reader created auxiliary state")
			}
		})
	}
}
func TestSourcesRejectUnknownAndChangingFormats(t *testing.T) {
	if _, err := (FileSource{Format: "deja-sync", ProducerVersion: "0.22.0"}).Discover(); err == nil {
		t.Fatal("guessed unknown version")
	}
	for _, data := range []string{`{"schema_version":2,"sessions":[]}`, `{"schema_version":1,"sessions":[],"extra":true}`, `{"schema_version":1,"sessions":[]} {}`} {
		path := filepath.Join(t.TempDir(), "fixture.json")
		os.WriteFile(path, []byte(data), 0600)
		source := FileSource{Paths: []string{path}, Format: "normalized"}
		artifacts, err := source.Discover()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := source.ReadSessions(context.Background(), artifacts[0]); err == nil {
			t.Fatal("accepted incompatible artifact")
		}
	}
	path := filepath.Join(t.TempDir(), "fixture.json")
	os.WriteFile(path, []byte(`{"schema_version":1,"sessions":[]}`), 0600)
	source := FileSource{Paths: []string{path}, Format: "normalized"}
	artifacts, _ := source.Discover()
	os.WriteFile(path, []byte(`changed`), 0600)
	if _, err := source.ReadSessions(context.Background(), artifacts[0]); err == nil {
		t.Fatal("accepted changed artifact")
	}
}
func TestNormalizedFixtureUnknownExitAndScopedCoverage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	a := NormalizedArtifact{SchemaVersion: 1, Sessions: []model.Session{{ID: "fixture", Commands: []model.SessionCommand{{ID: "command"}}, Coverage: model.Coverage{GitHistory: model.CoverageObservation{Level: model.CoverageComplete}}}}}
	data, _ := json.Marshal(a)
	os.WriteFile(path, data, 0600)
	source := FileSource{Paths: []string{path}, Format: "normalized"}
	artifacts, _ := source.Discover()
	ss, err := source.ReadSessions(context.Background(), artifacts[0])
	if err != nil {
		t.Fatal(err)
	}
	if ss[0].Commands[0].ExitCode != nil || ss[0].Coverage.GitHistory.Level != model.CoverageUnknown {
		t.Fatal("invented success/complete scope")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.ReadSessions(ctx, artifacts[0]); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestMergeRetainsSightingsAndRejectsConflictingIntent(t *testing.T) {
	a := model.Session{ID: "fixture", Provider: "deja-vu", HostID: "host", WorkingDir: "/fixture", StartedAt: timeAt(1), LastObservedAt: timeAt(2), Intents: []model.Intent{{ID: "old"}}, Sources: []model.SessionSource{{Path: "first.jsonl"}}}
	b := a
	b.LastObservedAt = timeAt(3)
	b.Intents = []model.Intent{{ID: "new"}}
	b.Sources = []model.SessionSource{{Path: "second.jsonl"}}
	merged, err := Merge(a, b)
	if err != nil || len(merged.Sources) != 2 || merged.Intents[0].ID != "new" || !merged.StartedAt.Equal(*timeAt(1)) {
		t.Fatal("lost batch provenance/latest observation")
	}
	b.LastObservedAt = timeAt(2)
	if _, err := Merge(a, b); err == nil {
		t.Fatal("invented ordering for equal-time tails")
	}
	a.Provider = "normalized-json"
	b.Provider = a.Provider
	b.Intents = []model.Intent{{ID: "old", Description: "conflicting definition"}}
	if _, err := Merge(a, b); err == nil {
		t.Fatal("guessed conflicting intent")
	}
}
