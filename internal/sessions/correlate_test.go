package sessions

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MTG-Thomas/spuro/internal/analyze"
	"github.com/MTG-Thomas/spuro/internal/config"
	gitbackend "github.com/MTG-Thomas/spuro/internal/git"
	"github.com/MTG-Thomas/spuro/internal/model"
)

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	prefix := []string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + os.DevNull, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "core.autocrlf=false", "-C", dir}
	cmd := exec.Command("git", append(prefix, args...)...)
	env := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			env = append(env, entry)
		}
	}
	cmd.Env = append(env, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture %v: %v %s", args, err, b)
	}
	return strings.TrimSpace(string(b))
}
func estate(t *testing.T) (string, *model.Result, *gitbackend.Runner) {
	t.Helper()
	root := t.TempDir()
	fixtureGit(t, root, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "parser.go"), []byte("package parser\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, root, "add", ".")
	fixtureGit(t, root, "commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(root, "parser_test.go"), []byte("package parser\n// RegressionWindowsSeparator\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, root, "add", ".")
	fixtureGit(t, root, "commit", "-qm", "regression fixture")
	if err := os.WriteFile(filepath.Join(root, "unfinished.go"), []byte("uncommitted fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := analyze.Scan(context.Background(), analyze.Options{Roots: []string{root}, Config: config.Defaults(), NoPlugins: true})
	if err != nil {
		t.Fatal(err)
	}
	g, err := gitbackend.New()
	if err != nil {
		t.Fatal(err)
	}
	return root, &r, g
}
func timeAt(n int) *time.Time { v := time.Date(2026, 9, 1, n, 0, 0, 0, time.UTC); return &v }
func intent(session, id, kind, path, literal string) model.Intent {
	return model.Intent{ID: id, SessionID: session, Explicit: true, Files: []string{path}, Completion: []model.CriterionGroup{{Mode: "ALL", Criteria: []model.CompletionCriterion{{ID: id + "-predicate", Kind: kind, Files: []string{path}, Literal: literal}}}}}
}
func TestIndependentCompletionDirtyAndCancellation(t *testing.T) {
	root, native, g := estate(t)
	s := model.Session{ID: "original", HostID: "fixture", WorkingDir: root, StartedAt: timeAt(1), EndedAt: timeAt(2), Coverage: model.UnknownCoverage(), Intents: []model.Intent{
		intent("original", "parser", "literal_in_file", "parser.go", "package parser"), intent("original", "test", "literal_in_file", "parser_test.go", "RegressionWindowsSeparator"), intent("original", "docs", "literal_in_file", "README.md", "regression documentation"), intent("original", "dirty", "literal_in_file", "unfinished.go", "uncommitted fixture")}}
	follow := model.Session{ID: "follow", HostID: "fixture", WorkingDir: root, StartedAt: timeAt(3), EndedAt: timeAt(4), Continues: []string{"original"}, Decisions: []model.IntentDecision{{SessionID: "original", IntentID: "docs", Actor: "user", Decision: "cancelled", Evidence: []string{"user-cancellation-fixture"}}}}
	engine := Engine{Git: g, Native: native, HostID: "fixture"}
	result := engine.Correlate(context.Background(), []model.Session{s, follow})
	a := result.Assessments[0]
	if a.Summary != model.SessionPartiallyCompleted || a.Intents[0].State != model.IntentCompleted || a.Intents[1].State != model.IntentCompleted || a.Intents[2].State != model.IntentSuperseded || a.Intents[3].State != model.IntentDirtyStateRemains {
		t.Fatalf("wrong intent states: %+v", a)
	}
	if len(a.Intents[3].RelatedFindings) == 0 || len(a.Intents[3].Evidence) == 0 {
		t.Fatal("missing native links")
	}
	found := false
	for _, f := range result.Findings {
		if f.Kind == "session_dirty_state_remains" && f.Severity == model.PreserveFirst {
			found = true
		}
	}
	if !found {
		t.Fatal("native preservation severity not retained")
	}
	// Cancellation is only one intent; unrelated docs criteria do not disappear.
	s.Intents[2].ID = "uncancelled"
	s.Intents[2].Completion[0].Criteria[0].ID = "uncancelled-predicate"
	result = engine.Correlate(context.Background(), []model.Session{s, follow})
	if result.Assessments[0].Intents[2].State == model.IntentSuperseded {
		t.Fatal("cancelled unrelated intent")
	}
}
func TestFailureContinuationAndBoundPassingCommand(t *testing.T) {
	root, native, g := estate(t)
	failure, success := 1, 0
	i := model.Intent{ID: "validate", SessionID: "a", Explicit: true, Completion: []model.CriterionGroup{{Mode: "ALL", Criteria: []model.CompletionCriterion{{ID: "test-pass", Kind: "command_passed", Commands: []string{"go test ./..."}}}}}}
	a := model.Session{ID: "a", HostID: "fixture", WorkingDir: root, StartedAt: timeAt(1), EndedAt: timeAt(2), Intents: []model.Intent{i}, Commands: []model.SessionCommand{{ID: "fail", IntentIDs: []string{"validate"}, Command: "go test ./...", WorkingDir: root, ExitCode: &failure, EndedAt: timeAt(2), Evidence: []string{"recorded-exit-1"}}}}
	e := Engine{Git: g, Native: native, HostID: "fixture"}
	r := e.Correlate(context.Background(), []model.Session{a})
	if r.Assessments[0].Intents[0].State != model.IntentFailedUnresolved || r.Assessments[0].Intents[0].Coverage.SessionHistory.Level != model.CoveragePartial {
		t.Fatal("invented abandonment/coverage")
	}
	b := model.Session{ID: "b", HostID: "fixture", WorkingDir: root, StartedAt: timeAt(3), EndedAt: timeAt(4), Continues: []string{"a"}}
	c := model.Session{ID: "c", HostID: "fixture", WorkingDir: root, StartedAt: timeAt(5), EndedAt: timeAt(6), Continues: []string{"b"}, Commands: []model.SessionCommand{{ID: "pass", IntentRefs: []model.IntentRef{{SessionID: "a", IntentID: "validate"}}, Command: "go test ./...", WorkingDir: root, ExitCode: &success, EndedAt: timeAt(6), Evidence: []string{"recorded-exit-0"}}}}
	r = e.Correlate(context.Background(), []model.Session{a, b, c})
	if r.Assessments[0].Intents[0].State != model.IntentCompleted {
		t.Fatalf("linked completion missing: %+v", r.Assessments[0])
	}
	c.Commands[0].IntentRefs = nil
	r = e.Correlate(context.Background(), []model.Session{a, b, c})
	if r.Assessments[0].Intents[0].State == model.IntentCompleted {
		t.Fatal("passing unrelated command completed intent")
	}
	c.Commands[0].IntentRefs = []model.IntentRef{{SessionID: "a", IntentID: "validate"}}
	c.HostID = "other-host"
	r = e.Correlate(context.Background(), []model.Session{a, b, c})
	if r.Assessments[0].Intents[0].State == model.IntentCompleted {
		t.Fatal("cross-host association guessed")
	}
}
func TestRebasedPatchCompletionAndChangedCoverage(t *testing.T) {
	root, native, g := estate(t)
	fixtureGit(t, root, "checkout", "-qb", "candidate")
	os.WriteFile(filepath.Join(root, "feature"), []byte("patch fixture\n"), 0600)
	fixtureGit(t, root, "add", "feature")
	fixtureGit(t, root, "commit", "-qm", "candidate")
	candidate := fixtureGit(t, root, "rev-parse", "HEAD")
	fixtureGit(t, root, "checkout", "-q", "main")
	os.WriteFile(filepath.Join(root, "different"), []byte("different history\n"), 0600)
	fixtureGit(t, root, "add", "different")
	fixtureGit(t, root, "commit", "-qm", "different")
	fixtureGit(t, root, "cherry-pick", candidate)
	fixtureGit(t, root, "branch", "-D", "candidate")
	fresh, err := analyze.Scan(context.Background(), analyze.Options{Roots: []string{root}, Config: config.Defaults(), NoPlugins: true})
	if err != nil {
		t.Fatal(err)
	}
	native = &fresh
	s := model.Session{ID: "rebase", HostID: "fixture", WorkingDir: root, Intents: []model.Intent{{ID: "patch", SessionID: "rebase", Completion: []model.CriterionGroup{{Mode: "ALL", Criteria: []model.CompletionCriterion{{ID: "patch-predicate", Kind: "patch_represented", ExpectedOID: candidate, ExpectedObjectFormat: "sha1"}}}}}}}
	e := Engine{Git: g, Native: native, HostID: "fixture"}
	r := e.Correlate(context.Background(), []model.Session{s})
	if r.Assessments[0].Intents[0].State != model.IntentSuperseded {
		t.Fatalf("missed patch equivalent: %+v", r.Assessments[0])
	}
	native.Repositories[0].ChangedDuringScan = true
	r = e.Correlate(context.Background(), []model.Session{s})
	if r.Assessments[0].Intents[0].State == model.IntentCompleted || r.Assessments[0].Intents[0].State == model.IntentSuperseded {
		t.Fatal("unstable native evidence completed intent")
	}
}

func TestStagedDeletionRetainsCompleteCheckoutObservation(t *testing.T) {
	root, _, g := estate(t)
	fixtureGit(t, root, "rm", "parser.go")
	native, err := analyze.Scan(context.Background(), analyze.Options{Roots: []string{root}, Config: config.Defaults(), NoPlugins: true})
	if err != nil {
		t.Fatal(err)
	}
	s := model.Session{ID: "deletion", HostID: "fixture", WorkingDir: root, EndedAt: timeAt(2), Intents: []model.Intent{{ID: "deleted-path", SessionID: "deletion", Files: []string{"parser.go"}, Explicit: true}}}
	engine := Engine{Git: g, Native: &native, HostID: "fixture"}
	r := engine.Correlate(context.Background(), []model.Session{s})
	if r.Assessments[0].Intents[0].State != model.IntentDirtyStateRemains || len(r.RelatedCheckouts) != 1 {
		t.Fatal("staged deletion lost")
	}
	found := false
	for _, change := range r.RelatedCheckouts[0].Staged {
		if change.Path == "parser.go" && strings.HasPrefix(change.Status, "D") {
			found = true
		}
	}
	if !found {
		t.Fatal("report dropped deletion intent/index state")
	}
}
