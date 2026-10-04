package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"spuro/internal/config"
	"spuro/internal/model"
	"strings"
	"testing"
)

func nativeResult() model.Result {
	return model.Result{SchemaVersion: 1, Repositories: []model.Repository{{ID: "repo-test", Path: "/source/repo", Complete: true, Checkouts: []model.Checkout{{Path: "/source/repo", Valid: true, StatusKnown: true}}, Evidence: []model.Evidence{}}}, Findings: []model.Finding{{ID: "native", Severity: model.PreserveFirst, Kind: "dirty_checkout"}}, Plugins: []model.PluginRun{}, Diagnostics: []model.Diagnostic{}}
}
func TestPluginFailureIsolationAndAuthority(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executables use POSIX shell")
	}
	tests := []struct{ name, body, status string }{{"valid", `if [ "$1" = manifest ]; then printf '%s' '{"protocol_version":1,"name":"test","version":"1","read_only":true}'; else printf '%s' '{"protocol_version":1,"observations":[{"repository":"/source/repo","kind":"clean","severity":"PRESERVE_FIRST","message":"plugin incorrectly claims clean"}]}'; fi`, "ok"}, {"incompatible", `printf '%s' '{"protocol_version":99,"name":"test","read_only":true}'`, "refused"}, {"mutation", `printf '%s' '{"protocol_version":1,"name":"test","read_only":false}'`, "refused"}, {"malformed", `printf '%s' 'not JSON'`, "failed"}, {"scan-malformed", `if [ "$1" = manifest ]; then printf '%s' '{"protocol_version":1,"name":"test","read_only":true}'; else printf '%s' 'not JSON'; fi`, "failed"}, {"timeout", `sleep 10`, "failed"}, {"crash", `exit 3`, "failed"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "plugin")
			if e := os.WriteFile(p, []byte("#!/bin/sh\n"+tt.body+"\n"), 0700); e != nil {
				t.Fatal(e)
			}
			cfg := config.Defaults()
			cfg.Plugins["test"] = config.Plugin{Command: p, ReadOnlyVerified: true, Documentation: "audited fixture", TimeoutSeconds: 1}
			r := nativeResult()
			Run(context.Background(), cfg, []string{"test"}, &r)
			if r.Plugins[0].Status != tt.status {
				t.Fatalf("run: %+v", r.Plugins)
			}
			if !r.Repositories[0].Checkouts[0].Valid || !r.Repositories[0].Complete || r.Findings[0].Severity != model.PreserveFirst {
				t.Fatal("plugin replaced native evidence")
			}
			if tt.name == "valid" {
				if len(r.Findings) != 2 || r.Findings[1].Severity != model.Info || r.Findings[1].Confidence != model.Low {
					t.Fatal("plugin promoted risk")
				}
				if r.Repositories[0].Evidence[0].Provider != "test" {
					t.Fatal("missing provenance")
				}
			}
		})
	}
}
func TestUnknownExecutableNotInvoked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	p := filepath.Join(dir, "unsafe")
	os.WriteFile(p, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700)
	cfg := config.Defaults()
	cfg.Plugins["test"] = config.Plugin{Command: p}
	r := nativeResult()
	Run(context.Background(), cfg, []string{"test"}, &r)
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("unverified executable invoked")
	}
	if r.Plugins[0].Status != "refused" {
		t.Fatal(r.Plugins)
	}
}
func TestUnavailable(t *testing.T) {
	cfg := config.Defaults()
	cfg.Plugins["test"] = config.Plugin{Command: "spuro-definitely-not-installed", ReadOnlyVerified: true, Documentation: "fixture"}
	r := nativeResult()
	Run(context.Background(), cfg, []string{"test"}, &r)
	if r.Plugins[0].Status != "unavailable" {
		t.Fatal(r.Plugins)
	}
}
func TestAdapterJSONAndManifest(t *testing.T) {
	var b strings.Builder
	if e := AdapterMain("gitwell", []string{"manifest"}, nil, &b); e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e := json.Unmarshal([]byte(b.String()), &m); e != nil || !m.ReadOnly || m.ProtocolVersion != 1 {
		t.Fatal(b.String())
	}
	data := []byte(`{"repos":[{"path":"/repo","sections":{"Stale Branches":[{"type":"stale_branch","branch":"feature","message":"old"}]}}]}`)
	o, e := NormalizeAdapter("gitwell", data)
	if e != nil || len(o) != 1 || o[0].Subject != "feature" {
		t.Fatalf("%+v %v", o, e)
	}
	data = []byte(`{"repos":[{"repo":"/repo","worktrees":[{"path":"/linked","branch":"feature","dirty":true}]}]}`)
	o, e = NormalizeAdapter("stalewood", data)
	if e != nil || len(o) != 1 || o[0].Checkout != "/linked" {
		t.Fatalf("%+v %v", o, e)
	}
}
