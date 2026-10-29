package analyze

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MTG-Thomas/spuro/internal/model"
)

func TestStashDeletionSnapshotEvidence(t *testing.T) {
	for _, separateIndex := range []bool{false, true} {
		name := "preserved-working-deletion"
		if separateIndex {
			name = "unpreserved-index-deletion"
		}
		t.Run(name, func(t *testing.T) {
			root := fixtureTempDir(t)
			repo := initRepo(t, root, name)
			commitFile(t, repo, "other.txt", "before\n", "second file")
			if err := os.Remove(filepath.Join(repo, "file.txt")); err != nil {
				t.Fatal(err)
			}
			if separateIndex {
				fixtureGit(t, repo, "add", "-u")
				put(t, repo, "other.txt", "working change distinct from index\n")
			}
			fixtureGit(t, repo, "stash", "push", "-m", "deletion snapshots")
			stash := fixtureGit(t, repo, "rev-parse", "refs/stash")
			// Preserve exactly the working snapshot, not the separate index snapshot.
			if err := os.Remove(filepath.Join(repo, "file.txt")); err != nil {
				t.Fatal(err)
			}
			if separateIndex {
				put(t, repo, "other.txt", "working change distinct from index\n")
			}
			fixtureGit(t, repo, "add", "-u")
			fixtureGit(t, repo, "commit", "-qm", "integrate working snapshot")
			before := snapshot(t, root)
			result := nativeScan(t, root)
			if !reflect.DeepEqual(before, snapshot(t, root)) {
				t.Fatal("scanner mutated fixture")
			}
			lineage := strandAt(t, repoAt(t, result, repo), stash)
			preserved, unpreserved := 0, 0
			for _, v := range lineage.StashFiles {
				if v.Kind != "stash-deletion" {
					continue
				}
				if len(v.RepresentedBy) > 0 {
					preserved++
				} else {
					unpreserved++
				}
			}
			wantUnpreserved := 0
			wantSeverity := model.Housekeeping
			if separateIndex {
				wantUnpreserved = 1
				wantSeverity = model.Review
			}
			if preserved != 1 || unpreserved != wantUnpreserved {
				t.Fatalf("deletion evidence lost or bound to wrong snapshot: %+v", lineage.StashFiles)
			}
			for _, f := range result.Findings {
				if f.Kind == "stash" && f.OID == stash {
					if f.Severity != wantSeverity {
						t.Fatalf("got %s, want %s: %+v", f.Severity, wantSeverity, f)
					}
					return
				}
			}
			t.Fatal("stash finding missing")
		})
	}
}
