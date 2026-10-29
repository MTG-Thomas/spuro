package analyze

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MTG-Thomas/spuro/internal/model"
)

func TestStashRenameRetainsFollowingVersions(t *testing.T) {
	root := fixtureTempDir(t)
	repo := initRepo(t, root, "stash-rename")
	fixtureGit(t, repo, "config", "diff.renames", "true")
	commitFile(t, repo, "z-last.txt", "before\n", "second file")
	fixtureGit(t, repo, "mv", "file.txt", unusualPath("a-renamed ü\nfile"))
	put(t, repo, "z-last.txt", "unique stashed version\n")
	fixtureGit(t, repo, "stash", "push", "-m", "rename and edit")
	stash := fixtureGit(t, repo, "rev-parse", "refs/stash")
	before := snapshot(t, root)
	result := nativeScan(t, root)
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("scanner mutated fixture")
	}
	lineage := strandAt(t, repoAt(t, result, repo), stash)
	want := map[string]bool{
		"stash-working\x00" + unusualPath("a-renamed ü\nfile"): false,
		"stash-index\x00" + unusualPath("a-renamed ü\nfile"):   false,
		"stash-working\x00z-last.txt":                          false,
		"stash-deletion\x00file.txt":                           false,
	}
	for _, v := range lineage.StashFiles {
		if _, ok := want[v.Kind+"\x00"+v.Path]; ok {
			want[v.Kind+"\x00"+v.Path] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing stash version %q: %+v", key, lineage.StashFiles)
		}
	}
}

func TestUnpreservedDeletionOnlyStashNeedsReview(t *testing.T) {
	root := fixtureTempDir(t)
	repo := initRepo(t, root, "stash-deletion")
	if err := os.Remove(filepath.Join(repo, "file.txt")); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, repo, "stash", "push", "-m", "unique deletion intent")
	stash := fixtureGit(t, repo, "rev-parse", "refs/stash")
	before := snapshot(t, root)
	result := nativeScan(t, root)
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("scanner mutated fixture")
	}
	for _, finding := range result.Findings {
		if finding.Kind == "stash" && finding.OID == stash {
			if finding.Severity != model.Review {
				t.Fatalf("unpreserved deletion classified %s, want REVIEW: %+v", finding.Severity, finding)
			}
			if len(finding.Paths) != 1 || finding.Paths[0] != "file.txt" {
				t.Fatalf("deletion path absent: %+v", finding)
			}
			return
		}
	}
	t.Fatal("stash finding missing")
}
