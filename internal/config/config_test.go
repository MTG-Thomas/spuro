package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigIndependentSwitches(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("[analysis]\npatch_equivalence=false\n[plugins.test]\nenabled=true\ncommand='example'\n"), 0600)
	c, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	if c.PatchEnabled() || !c.UnreachableEnabled() || !c.Plugins["test"].Enabled {
		t.Fatalf("config: %+v", c)
	}
}
func TestGlob(t *testing.T) {
	for _, path := range []string{"node_modules/", "a/node_modules/", "a/node_modules/repo/"} {
		r, e := Glob("**/node_modules/**")
		if e != nil || !r.MatchString(path) {
			t.Fatalf("glob misses %q", path)
		}
	}
}
