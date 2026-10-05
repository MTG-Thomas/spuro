package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/MTG-Thomas/spuro/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestCLI(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--help"}, {"plugins", "--json"}} {
		var out, errout bytes.Buffer
		if e := run(context.Background(), args, &out, &errout); e != nil || out.Len() == 0 {
			t.Fatalf("%v: %v", args, e)
		}
	}
	for _, args := range [][]string{{"scan"}, {"scan", "--max-workers", "0"}, {"scan", "--unknown"}, {"unknown"}} {
		if e := run(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
func TestFlagsAfterRootAndOutputSafety(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(t.TempDir(), "result.json")
	var stdout, stderr bytes.Buffer
	if e := run(context.Background(), []string{"scan", root, "--json", "--no-plugins", "--max-workers=2", "--output", output}, &stdout, &stderr); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(output)
	if e != nil {
		t.Fatal(e)
	}
	var r model.Result
	if e = json.Unmarshal(b, &r); e != nil || r.SchemaVersion != 1 || r.Scan.MaxWorkers != 2 {
		t.Fatalf("%v %+v", e, r)
	}
	if e := run(context.Background(), []string{"scan", root, "--output", filepath.Join(root, "report")}, &stdout, &stderr); e == nil {
		t.Fatal("CLI allowed output inside source")
	}
}

func TestSessionCLIUnavailableAndArtifactOutputSafety(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(t.TempDir(), "session-fixture.json")
	data := []byte(`{"schema_version":1,"sessions":[]}`)
	if err := os.WriteFile(artifact, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"sessions", root, "--json"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("provider skipped")) {
		t.Fatal("missing provider diagnostic")
	}
	for _, output := range []string{artifact, filepath.Join(root, "report.json")} {
		if err := run(context.Background(), []string{"sessions", root, "--source", artifact, "--output", output}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatal("unsafe output accepted")
		}
	}
	link := filepath.Join(filepath.Dir(artifact), "alias.json")
	if err := os.Symlink(artifact, link); err == nil {
		if err := run(context.Background(), []string{"sessions", root, "--source", artifact, "--output", link}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatal("source symlink overwritten")
		}
	}
	got, _ := os.ReadFile(artifact)
	if !bytes.Equal(data, got) {
		t.Fatal("artifact mutated")
	}
}
