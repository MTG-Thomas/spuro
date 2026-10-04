package git

import (
	"context"
	"github.com/MTG-Thomas/spuro/internal/model"
	"strings"
	"testing"
)

func TestReadOnlyGate(t *testing.T) {
	for _, args := range [][]string{{"gc"}, {"prune"}, {"reflog", "expire", "--all"}, {"worktree", "prune"}, {"clean", "-fd"}, {"reset", "--hard"}, {"checkout", "main"}, {"switch", "main"}, {"merge", "x"}, {"rebase", "main"}, {"cherry-pick", "HEAD"}, {"stash", "drop"}, {"branch", "-D", "x"}, {"update-ref", "refs/heads/main", "x"}, {"hash-object", "-w", "--stdin", "--no-filters"}, {"fsck", "--unreachable", "--lost-found"}, {"diff", "--no-ext-diff", "--no-textconv", "--output=x"}, {"symbolic-ref", "HEAD", "refs/heads/new"}, {"config", "--set", "foo", "bar"}} {
		if Allowed(args) {
			t.Errorf("allowed mutation %v", args)
		}
		r := &Runner{Binary: "must-not-execute"}
		z := r.Run(context.Background(), "", args...)
		if z.Err == nil || !strings.Contains(z.Err.Error(), "gate refused") {
			t.Errorf("mutation did not stop at gate: %v", args)
		}
	}
}
func TestNULRenameAndNewline(t *testing.T) {
	c := model.Checkout{}
	raw := "# branch.head main\x00# branch.oid " + strings.Repeat("a", 40) + "\x002 R. N... 100644 100644 100644 " + strings.Repeat("a", 40) + " " + strings.Repeat("b", 40) + " R100 new ü\nname\x00old ü\nname\x00? untracked\nname\x00"
	if e := Status([]byte(raw), &c); e != nil {
		t.Fatal(e)
	}
	if len(c.Staged) != 1 || c.Staged[0].Path != "new ü\nname" || c.Staged[0].OriginalPath != "old ü\nname" {
		t.Fatalf("rename lost: %+v", c)
	}
	if c.Untracked[0] != "untracked\nname" {
		t.Fatal("untracked newline lost")
	}
}
func TestRemoteCredentialsRedacted(t *testing.T) {
	s := RedactURL("https://user:secret@example.test/x?access_token=token#sensitive")
	if strings.Contains(s, "secret") || strings.Contains(s, "token") || strings.Contains(s, "sensitive") {
		t.Fatal(s)
	}
}
