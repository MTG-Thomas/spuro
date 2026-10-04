package analyze

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/config"
	"github.com/MTG-Thomas/spuro/internal/model"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These deliberately mutated repositories are disposable test fixtures only.
// No production Git mutation goes through this helper or the scanner runner.
func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "-c", "gc.auto=0", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.test", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.test")
	b, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("fixture git %v: %v %s", args, e, b)
	}
	return strings.TrimSuffix(string(b), "\n")
}
func put(t *testing.T, dir, path, text string) {
	t.Helper()
	p := filepath.Join(dir, path)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
}
func initRepo(t *testing.T, root, name string) string {
	t.Helper()
	p := filepath.Join(root, name)
	if e := os.MkdirAll(p, 0700); e != nil {
		t.Fatal(e)
	}
	fixtureGit(t, p, "init", "-q", "-b", "main")
	put(t, p, "file.txt", "base\n")
	fixtureGit(t, p, "add", ".")
	fixtureGit(t, p, "commit", "-qm", "base")
	return p
}
func commitFile(t *testing.T, dir, path, text, msg string) string {
	t.Helper()
	put(t, dir, path, text)
	fixtureGit(t, dir, "add", path)
	fixtureGit(t, dir, "commit", "-qm", msg)
	return fixtureGit(t, dir, "rev-parse", "HEAD")
}
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		fi, e := d.Info()
		if e != nil {
			return e
		}
		if d.IsDir() {
			out[p] = fi.Mode().String()
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		out[p] = fmt.Sprintf("%s %s %x", fi.Mode(), fi.ModTime().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), sha256.Sum256(b))
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func nativeScan(t *testing.T, roots ...string) model.Result {
	t.Helper()
	r, e := Scan(context.Background(), Options{Roots: roots, Config: config.Defaults(), NoPlugins: true})
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range r.Diagnostics {
		t.Logf("diagnostic: %+v", d)
	}
	return r
}
func repoAt(t *testing.T, r model.Result, path string) model.Repository {
	t.Helper()
	for _, repo := range r.Repositories {
		if repo.Path == path {
			return repo
		}
		for _, c := range repo.Checkouts {
			if c.Path == path {
				return repo
			}
		}
	}
	t.Fatalf("no repository at %q", path)
	return model.Repository{}
}
func strandAt(t *testing.T, repo model.Repository, oid string) model.Lineage {
	t.Helper()
	for _, l := range repo.Lineages {
		if l.TipOID == oid {
			return l
		}
	}
	t.Fatalf("no lineage %s", oid)
	return model.Lineage{}
}
func hasFinding(r model.Result, kind string, oid string) bool {
	for _, f := range r.Findings {
		if f.Kind == kind && (oid == "" || f.OID == oid) {
			return true
		}
	}
	return false
}
func TestEstateDiscoveryStatusAndReadOnly(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "normal space ü\nrepo")
	nested := initRepo(t, repo, "nested")
	_ = nested
	linked := filepath.Join(root, "linked ü\ncheckout")
	fixtureGit(t, repo, "worktree", "add", "-q", "-b", "linked", linked)
	external := filepath.Join(t.TempDir(), "external")
	fixtureGit(t, repo, "worktree", "add", "-q", "--detach", external)
	bare := filepath.Join(root, "bare.git")
	fixtureGit(t, root, "clone", "--bare", "-q", repo, bare)
	separate := filepath.Join(root, "indirection")
	os.MkdirAll(separate, 0700)
	fixtureGit(t, separate, "init", "-q", "-b", "main", "--separate-git-dir", filepath.Join(root, "admin"))
	commitFile(t, separate, "x", "x\n", "x")
	excluded := initRepo(t, filepath.Join(root, "node_modules"), "excluded")
	_ = excluded
	broken := filepath.Join(root, "broken")
	put(t, broken, ".git", "gitdir: /definitely/not/a/gitdir\n")
	put(t, broken, "important.txt", "do not lose\n")
	put(t, repo, "file.txt", "modified\n")
	put(t, repo, "staged ü\nfile", "staged\n")
	fixtureGit(t, repo, "add", "staged ü\nfile")
	put(t, linked, "untracked ü\nfile", "unique untracked\n")
	before := snapshot(t, root)
	outsideBefore := snapshot(t, external)
	r := nativeScan(t, root)
	after := snapshot(t, root)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(outsideBefore, snapshot(t, external)) {
		t.Fatal("scan mutated Git metadata or checkout contents")
	}
	if len(r.Repositories) != 4 {
		t.Fatalf("repositories=%d want 4 (normal,nested,bare,indirection deduplicated with its admin)", len(r.Repositories))
	}
	nr := repoAt(t, r, repo)
	if len(nr.Checkouts) != 3 {
		t.Fatalf("checkouts=%d want 3", len(nr.Checkouts))
	}
	found := false
	for _, c := range nr.Checkouts {
		if c.Path == repo {
			if len(c.Modified) != 1 || len(c.Staged) != 1 || !c.StatusKnown {
				t.Fatalf("dirty state: %+v", c)
			}
		}
		if c.Path == linked {
			found = true
			if len(c.Untracked) != 1 || c.Untracked[0] != "untracked ü\nfile" {
				t.Fatalf("untracked parsing: %+v", c)
			}
		}
	}
	if !found || !hasFinding(r, "untracked_unique", "") || !hasFinding(r, "dirty_checkout", "") {
		t.Fatalf("missing dirty findings: %+v", r.Findings)
	}
}
func TestBranchUpstreamsAndEquivalence(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "repo")
	base := fixtureGit(t, repo, "rev-parse", "HEAD")
	fixtureGit(t, repo, "remote", "add", "origin", "https://example.test/repo.git")
	fixtureGit(t, repo, "update-ref", "refs/remotes/origin/main", base)
	fixtureGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	fixtureGit(t, repo, "branch", "--set-upstream-to=origin/main", "main")
	fixtureGit(t, repo, "checkout", "-qb", "feature")
	unique := commitFile(t, repo, "unique", "unique\n", "unique feature")
	fixtureGit(t, repo, "branch", "--set-upstream-to=origin/main", "feature")
	fixtureGit(t, repo, "branch", "gone", unique)
	fixtureGit(t, repo, "update-ref", "refs/remotes/origin/gone", base)
	fixtureGit(t, repo, "branch", "--set-upstream-to=origin/gone", "gone")
	fixtureGit(t, repo, "update-ref", "-d", "refs/remotes/origin/gone")
	fixtureGit(t, repo, "branch", "no-upstream", base)
	// Remove the alias so feature's own ref cannot claim to preserve itself.
	fixtureGit(t, repo, "branch", "-D", "gone")
	fixtureGit(t, repo, "checkout", "-q", "main")
	fixtureGit(t, repo, "branch", "gone", base)
	fixtureGit(t, repo, "config", "branch.gone.remote", "origin")
	fixtureGit(t, repo, "config", "branch.gone.merge", "refs/heads/gone")
	fixtureGit(t, repo, "checkout", "-qb", "original", base)
	original := commitFile(t, repo, "patch", "same patch\n", "original patch")
	fixtureGit(t, repo, "checkout", "-q", "main")
	commitFile(t, repo, "independent", "other\n", "independent base")
	fixtureGit(t, repo, "cherry-pick", "--no-edit", original)
	picked := fixtureGit(t, repo, "rev-parse", "HEAD")
	fixtureGit(t, repo, "update-ref", "refs/remotes/origin/main", picked)
	// A metadata-only commit has the exact same snapshot as main.
	fixtureGit(t, repo, "checkout", "-qb", "tree-equivalent")
	fixtureGit(t, repo, "commit", "--allow-empty", "-qm", "different metadata same tree")
	tree := fixtureGit(t, repo, "rev-parse", "HEAD")
	fixtureGit(t, repo, "checkout", "-q", "main")
	r := nativeScan(t, root)
	nr := repoAt(t, r, repo)
	if nr.Primary != "refs/remotes/origin/main" {
		t.Fatalf("primary=%s", nr.Primary)
	}
	for _, b := range nr.Branches {
		if b.Name == "refs/heads/feature" && (!b.CountsKnown || b.Ahead != 1 || b.RemoteReachable) {
			t.Fatalf("feature: %+v", b)
		}
		if b.Name == "refs/heads/gone" && !b.UpstreamGone {
			t.Fatalf("missing deleted upstream: %+v", b)
		}
	}
	if l := strandAt(t, nr, original); l.Equivalence != model.PatchEquivalent {
		t.Fatalf("cherry-picked branch not equivalent: %+v", l)
	}
	if l := strandAt(t, nr, tree); l.Equivalence != model.TreeEquivalent {
		t.Fatalf("same tree not recognized: %+v", l)
	}
	if l := strandAt(t, nr, unique); l.Equivalence != model.Unique {
		t.Fatalf("unique branch misclassified: %+v", l)
	}
	if !hasFinding(r, "unpushed_unique_branch", unique) {
		t.Fatal("missing unique unpushed branch")
	}
}
func TestReflogUnreachableDetachedAndDroppedStash(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "repo")
	base := fixtureGit(t, repo, "rev-parse", "HEAD")
	fixtureGit(t, repo, "checkout", "-qb", "ephemeral")
	reflog := commitFile(t, repo, "reflog", "reflog-only\n", "forgotten reflog")
	fixtureGit(t, repo, "checkout", "-q", "main")
	fixtureGit(t, repo, "branch", "-D", "ephemeral")
	// commit-tree makes an object with no ref or reflog, exercising fsck discovery.
	put(t, repo, "unreachable", "object only\n")
	fixtureGit(t, repo, "add", "unreachable")
	tree := fixtureGit(t, repo, "write-tree")
	unreachable := fixtureGit(t, repo, "commit-tree", tree, "-p", base, "-m", "unreachable fixture")
	fixtureGit(t, repo, "reset", "--hard", "-q", base)
	linked := filepath.Join(root, "detached")
	fixtureGit(t, repo, "worktree", "add", "-q", "--detach", linked)
	detached := commitFile(t, linked, "detached", "unique detached\n", "detached work")
	put(t, repo, "file.txt", "stash tracked\n")
	put(t, repo, "forgotten ü\nfile", "unique dropped stash payload\n")
	fixtureGit(t, repo, "stash", "push", "-u", "-m", "forgotten stash")
	dropped := fixtureGit(t, repo, "rev-parse", "refs/stash")
	fixtureGit(t, repo, "stash", "drop")
	before := snapshot(t, root)
	r := nativeScan(t, root)
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("archaeology mutated repository")
	}
	nr := repoAt(t, r, repo)
	if l := strandAt(t, nr, reflog); l.Reachability != model.ReflogOnly {
		t.Fatalf("reflog classification: %+v", l)
	}
	if l := strandAt(t, nr, unreachable); l.Reachability != model.Unreachable {
		t.Fatalf("unreachable classification: %+v", l)
	}
	if !hasFinding(r, "detached_unique_head", detached) {
		t.Fatal("missing detached risk")
	}
	l := strandAt(t, nr, dropped)
	if !l.StashLike || l.UntrackedParent == "" {
		t.Fatalf("stash structure lost: %+v", l)
	}
	found := false
	for _, v := range l.StashFiles {
		if v.Path == "forgotten ü\nfile" && len(v.DurableCopies) == 0 {
			found = true
		}
	}
	if !found || !hasFinding(r, "dropped_stash_candidate", dropped) {
		t.Fatalf("dropped stash risk missing: %+v", l)
	}
}
func TestConflictedCheckout(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "conflict")
	fixtureGit(t, repo, "checkout", "-qb", "other")
	commitFile(t, repo, "file.txt", "other\n", "other")
	fixtureGit(t, repo, "checkout", "-q", "main")
	commitFile(t, repo, "file.txt", "main\n", "main")
	cmd := exec.Command("git", "-c", "commit.gpgsign=false", "-C", repo, "merge", "other")
	if cmd.Run() == nil {
		t.Fatal("fixture expected conflict")
	}
	before := snapshot(t, root)
	r := nativeScan(t, root)
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("conflict scan mutated repository")
	}
	nr := repoAt(t, r, repo)
	c := nr.Checkouts[0]
	if len(c.Conflicted) != 1 || len(c.Index) != 3 || !hasFinding(r, "conflicted_checkout", "") {
		t.Fatalf("conflict stages lost: %+v", c)
	}
}
func TestCrossClonePreservationAndStableIDs(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "first")
	commitFile(t, repo, "durable", "preserved content\n", "durable")
	second := filepath.Join(root, "second")
	fixtureGit(t, root, "clone", "-q", repo, second)
	put(t, second, "untracked", "preserved content\n")
	one := nativeScan(t, root)
	two := nativeScan(t, root)
	ids := func(r model.Result) []string {
		out := []string{}
		for _, f := range r.Findings {
			out = append(out, f.ID)
		}
		return out
	}
	if !reflect.DeepEqual(ids(one), ids(two)) {
		t.Fatal("finding IDs changed without state changes")
	}
	for _, f := range one.Findings {
		if f.Kind == "untracked_unique" {
			t.Fatal("identical durable blob in another clone was ignored")
		}
	}
}
func TestRebasedEquivalenceAndCurrentStash(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "rebase")
	base := fixtureGit(t, repo, "rev-parse", "HEAD")
	fixtureGit(t, repo, "checkout", "-qb", "before")
	old := commitFile(t, repo, "rebased-file", "patch\n", "feature")
	fixtureGit(t, repo, "branch", "after")
	fixtureGit(t, repo, "checkout", "-q", "main")
	newBase := commitFile(t, repo, "base-file", "other\n", "new base")
	fixtureGit(t, repo, "checkout", "-q", "after")
	fixtureGit(t, repo, "rebase", "--onto", newBase, base, "after")
	rebased := fixtureGit(t, repo, "rev-parse", "HEAD")
	if old == rebased {
		t.Fatal("fixture not rebased")
	}
	fixtureGit(t, repo, "checkout", "-q", "main")
	put(t, repo, "stash-only", "current stash\n")
	fixtureGit(t, repo, "stash", "push", "-u", "-m", "current")
	stash := fixtureGit(t, repo, "rev-parse", "refs/stash")
	r := nativeScan(t, root)
	nr := repoAt(t, r, repo)
	if l := strandAt(t, nr, old); l.Equivalence != model.PatchEquivalent {
		t.Fatalf("rebased work not represented: %+v", l)
	}
	if len(nr.Stashes) != 1 || nr.Stashes[0].OID != stash || !hasFinding(r, "stash", stash) {
		t.Fatal("current stash missing")
	}
}
func TestStaleRegistrationAndUnbornRepo(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "registered")
	path := filepath.Join(root, "missing")
	fixtureGit(t, repo, "worktree", "add", "-q", "--detach", path)
	if e := os.RemoveAll(path); e != nil {
		t.Fatal(e)
	}
	empty := filepath.Join(root, "empty")
	os.MkdirAll(empty, 0700)
	fixtureGit(t, empty, "init", "-q", "-b", "main")
	r := nativeScan(t, root)
	if !hasFinding(r, "stale_worktree", "") {
		t.Fatal("missing worktree registration ignored")
	}
	nr := repoAt(t, r, empty)
	if len(nr.Checkouts) != 1 || !nr.Checkouts[0].StatusKnown {
		t.Fatalf("unborn state unknown: %+v", nr)
	}
}
func TestStagedVersionPreservationRisk(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "staged")
	put(t, repo, "file.txt", "unique staged content\n")
	fixtureGit(t, repo, "add", "file.txt")
	put(t, repo, "file.txt", "base\n")
	r := nativeScan(t, root)
	found := false
	for _, f := range r.Findings {
		if f.Kind == "dirty_checkout" && f.Severity == model.PreserveFirst {
			found = true
		}
	}
	if !found {
		t.Fatal("unique index-only bytes were demoted")
	}
}
func TestTrackedStashPatchEquivalentDespiteDifferentBlob(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "stash-rebased")
	body := strings.Repeat("unchanged context\n", 10) + "base\n"
	base := commitFile(t, repo, "file.txt", body, "larger base")
	put(t, repo, "file.txt", body+"feature\n")
	fixtureGit(t, repo, "stash", "push", "-m", "tracked stash")
	stash := fixtureGit(t, repo, "rev-parse", "refs/stash")
	fixtureGit(t, repo, "stash", "drop")
	put(t, repo, "file.txt", "prefix\n"+body)
	fixtureGit(t, repo, "add", "file.txt")
	fixtureGit(t, repo, "commit", "-qm", "new context")
	commitFile(t, repo, "file.txt", "prefix\n"+body+"feature\n", "same feature patch")
	r := nativeScan(t, root)
	nr := repoAt(t, r, repo)
	l := strandAt(t, nr, stash)
	if l.MergeBase != base {
		t.Fatal("unexpected stash base")
	}
	represented := false
	for _, v := range l.StashFiles {
		if v.Path == "file.txt" && len(v.DurableCopies) == 0 && len(v.RepresentedBy) > 0 {
			represented = true
		}
	}
	if !represented {
		t.Fatalf("stash patch was not correlated: %+v", l)
	}
	for _, f := range r.Findings {
		if f.OID == stash && f.Severity == model.PreserveFirst {
			t.Fatal("equivalent tracked stash over-ranked")
		}
	}
}
func TestSHA256Objects(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "sha256")
	os.MkdirAll(repo, 0700)
	cmd := exec.Command("git", "-C", repo, "init", "-q", "-b", "main", "--object-format=sha256")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Skipf("Git lacks SHA-256 support: %s", b)
	}
	oid := commitFile(t, repo, "file", "SHA256 fixture\n", "sha256")
	put(t, repo, "untracked", "untracked SHA256\n")
	r := nativeScan(t, root)
	nr := repoAt(t, r, repo)
	if nr.ObjectFormat != "sha256" || len(oid) != 64 || nr.Commits[0].OID != oid || !hasFinding(r, "untracked_unique", "") {
		t.Fatalf("SHA256 state: %+v", nr)
	}
}
func TestWeakNamespaceRefsGroupIntoTips(t *testing.T) {
	root := t.TempDir()
	repo := initRepo(t, root, "weak")
	fixtureGit(t, repo, "checkout", "-qb", "temporary")
	one := commitFile(t, repo, "one", "one\n", "one")
	two := commitFile(t, repo, "two", "two\n", "two")
	fixtureGit(t, repo, "update-ref", "refs/jj/keep/"+one, one)
	fixtureGit(t, repo, "update-ref", "refs/jj/keep/"+two, two)
	fixtureGit(t, repo, "checkout", "-q", "main")
	fixtureGit(t, repo, "branch", "-D", "temporary")
	r := nativeScan(t, root)
	nr := repoAt(t, r, repo)
	tips := 0
	for _, l := range nr.Lineages {
		if l.Kind == "unknown_ref" {
			tips++
			if l.TipOID != two {
				t.Fatal("ancestral keep ref became its own finding")
			}
		}
	}
	if tips != 1 {
		t.Fatalf("got %d weak namespace tips", tips)
	}
}
