package report

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"spuro/internal/model"
	"strings"
	"testing"
)

func TestOutputCannotModifyRepositories(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	r := model.Result{Scan: model.Scan{Roots: []string{root}}, Repositories: []model.Repository{{CommonGitDir: filepath.Join(root, ".git"), Checkouts: []model.Checkout{{Path: root}}}}}
	render := func(w io.Writer) error { _, e := io.WriteString(w, "report\n"); return e }
	if e := WriteOutside(filepath.Join(root, "report.json"), &r, render); e == nil {
		t.Fatal("wrote into root")
	}
	if e := WriteOutside(filepath.Join(outside, "report.json"), &r, render); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(outside, "redirect")
	if e := os.Symlink(root, link); e == nil {
		if e := WriteOutside(filepath.Join(link, "report.json"), &r, render); e == nil {
			t.Fatal("wrote via symlink")
		}
	}
	if _, e := os.Stat(filepath.Join(root, "report.json")); !os.IsNotExist(e) {
		t.Fatal("report exists inside source")
	}
}
func TestJSONCollections(t *testing.T) {
	r := model.Result{SchemaVersion: 1, Repositories: []model.Repository{{Checkouts: []model.Checkout{{}}}}}
	var b bytes.Buffer
	if e := JSON(&b, &r); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(b.String(), "null") {
		t.Fatal("null collection in JSON")
	}
}
