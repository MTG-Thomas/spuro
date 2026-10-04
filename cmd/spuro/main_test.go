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
