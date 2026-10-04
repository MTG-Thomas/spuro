package analyze

import (
	"encoding/json"
	"os"
	"spuro/internal/model"
	"testing"
)

// Optional external acceptance artifacts keep host paths and scanned private
// metadata out of this repository. This verifies an already captured scan.
func TestAcceptanceReport(t *testing.T) {
	reportPath := os.Getenv("SPURO_ACCEPTANCE_REPORT")
	baselinePath := os.Getenv("SPURO_ACCEPTANCE_BASELINE")
	if reportPath == "" || baselinePath == "" {
		t.Skip("set SPURO_ACCEPTANCE_REPORT and SPURO_ACCEPTANCE_BASELINE for real-estate acceptance")
	}
	var r model.Result
	var baseline struct {
		MinimumRepositories int `json:"minimum_repositories"`
		Required            []struct {
			Kind     string         `json:"kind"`
			OID      string         `json:"oid"`
			Path     string         `json:"path"`
			Severity model.Severity `json:"severity"`
		} `json:"required"`
	}
	for p, dest := range map[string]any{reportPath: &r, baselinePath: &baseline} {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(b, dest); e != nil {
			t.Fatal(e)
		}
	}
	if r.SchemaVersion != 1 || len(r.Repositories) < baseline.MinimumRepositories {
		t.Fatal("unexpected estate/schema shape")
	}
	seen := map[string]bool{}
	for _, repo := range r.Repositories {
		if seen[repo.CommonGitDir] {
			t.Fatal("shared Git directory counted twice")
		}
		seen[repo.CommonGitDir] = true
	}
	for _, expected := range baseline.Required {
		found := false
		for _, f := range r.Findings {
			if f.Kind != expected.Kind || f.Severity != expected.Severity || expected.OID != "" && f.OID != expected.OID {
				continue
			}
			if expected.Path != "" {
				for _, path := range f.Paths {
					if path == expected.Path {
						found = true
					}
				}
			} else {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing acceptance finding %+v", expected)
		}
	}
	equiv := 0
	for _, f := range r.Findings {
		if f.Kind == "tree_equivalent" || f.Kind == "patch_equivalent" || f.Kind == "exactly_preserved" {
			equiv++
		}
	}
	if equiv == 0 {
		t.Fatal("no preserved history collapsed")
	}
}
