package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MTG-Thomas/spuro/internal/model"
	"github.com/MTG-Thomas/spuro/internal/sessions"
)

func TestOfflineReconcileNoExecutablesAndProtectedInput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "snapshot.json")
	r := sessions.Result{Format: "spuro-sessions", SchemaVersion: 1, Sessions: []model.Session{{ID: "one", HostID: "host", Intents: []model.Intent{{ID: "intent", Description: "private optional idea", Files: []string{"candidate.go"}}}}}, Assessments: []model.DerivedSessionAssessment{{SessionID: "one", Intents: []model.IntentAssessment{{IntentID: "intent", State: model.IntentUnknown}}}}}
	body, _ := json.Marshal(r)
	if err := os.WriteFile(input, body, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // No Git or provider executable available.
	var b bytes.Buffer
	if err := run(context.Background(), []string{"sessions", "reconcile", "--input", input, "--json", "--no-transcript-text"}, &b, &b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "private optional idea") || !strings.Contains(b.String(), "CONCRETE_CANDIDATE") || !strings.Contains(b.String(), "source_sha256") {
		t.Fatal("bad offline reconciliation")
	}
	if err := run(context.Background(), []string{"sessions", "reconcile", "--input", input, "--output", input}, &b, &b); err == nil {
		t.Fatal("input overwrite allowed")
	}
	after, _ := os.ReadFile(input)
	if !bytes.Equal(after, body) {
		t.Fatal("input changed")
	}
}
func TestOfflineReconcileRejectsUnknownPrivateSchema(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private.json")
	if err := os.WriteFile(p, []byte(`{"intent_candidates":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := runSessionReconcile(context.Background(), []string{"--input", p}, &b); err == nil {
		t.Fatal("private schema guessed")
	}
}

func TestOfflineReconcileAmbiguousIdentityRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ambiguous.json")
	r := sessions.Result{Format: "spuro-sessions", SchemaVersion: 1, Sessions: []model.Session{{ID: "same"}, {ID: "same"}}}
	body, _ := json.Marshal(r)
	if err := os.WriteFile(p, body, 0600); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := runSessionReconcile(context.Background(), []string{"--input", p}, &b); err == nil {
		t.Fatal("ambiguous session identity accepted")
	}
}

func TestOfflineSchemaSightingRegression(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	input := "testdata/schema-sighting.json"
	before, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := runSessionReconcile(context.Background(), []string{"--input", input, "--json"}, &b); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Queue sessions.UnknownQueue `json:"unknown_reconciliation"`
	}
	if err := json.Unmarshal(b.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Queue.Groups) != 1 || len(result.Queue.Groups[0].Members) != 2 || len(result.Queue.Groups[0].Suggestions) != 0 || result.Queue.Groups[0].Priority == "FOLLOW_UP_CANDIDATE" {
		t.Fatal("schema sighting treated as later independent activity")
	}
	after, _ := os.ReadFile(input)
	if !bytes.Equal(before, after) {
		t.Fatal("input mutated")
	}
}
