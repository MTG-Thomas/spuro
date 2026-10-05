package sessions

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MTG-Thomas/spuro/internal/discovery"
	gitbackend "github.com/MTG-Thomas/spuro/internal/git"
	"github.com/MTG-Thomas/spuro/internal/model"
	"github.com/MTG-Thomas/spuro/internal/report"
)

type Result struct {
	NativeObservation model.Scan       `json:"native_observation"`
	RelatedCheckouts  []model.Checkout `json:"related_checkouts"`

	Format string `json:"format"`

	RelatedNativeFindings []model.Finding `json:"related_native_findings"`

	SchemaVersion int                              `json:"schema_version"`
	Version       string                           `json:"version"`
	HostID        string                           `json:"host_id"`
	ObservedAt    time.Time                        `json:"observed_at"`
	Sources       []SourceArtifact                 `json:"sources"`
	Sessions      []model.Session                  `json:"sessions"`
	Assessments   []model.DerivedSessionAssessment `json:"session_assessments"`
	Findings      []model.Finding                  `json:"session_findings"`
	Evidence      []model.Evidence                 `json:"evidence"`
	Diagnostics   []model.Diagnostic               `json:"diagnostics"`
	Native        *model.Result                    `json:"native_scan,omitempty"`
}

type Engine struct {
	children map[string][]model.Session

	Git    *gitbackend.Runner
	Native *model.Result
	HostID string
	cache  map[string]observation
}
type observation struct {
	known, satisfied, equivalent bool
	evidence                     []model.Evidence
	reason                       string
}

func canonical(path string) string {
	p, err := discovery.Canonical(path)
	if err != nil {
		return ""
	}
	return p
}
func (e *Engine) associate(s model.Session) (*model.Repository, *model.Checkout) {
	if s.HostID == "" || s.HostID != e.HostID || !filepath.IsAbs(s.WorkingDir) {
		return nil, nil
	}
	p := canonical(s.WorkingDir)
	if p == "" {
		return nil, nil
	}
	var repo *model.Repository
	var checkout *model.Checkout
	longest := 0
	for i := range e.Native.Repositories {
		r := &e.Native.Repositories[i]
		for j := range r.Checkouts {
			c := &r.Checkouts[j]
			root := canonical(c.Path)
			if root != "" && c.Valid && report.Within(p, root) && len(root) > longest {
				repo = r
				checkout = c
				longest = len(root)
			}
		}
	}
	return repo, checkout
}
func fact(operation, subject, detail string) model.Evidence {
	return model.Evidence{ID: model.ID("session-evidence-", operation, subject, detail), Provider: "git", Operation: operation, Fact: detail, Subject: subject}
}
func safeOID(oid, format string) bool {
	n := 40
	if format == "sha256" {
		n = 64
	} else if format != "sha1" {
		return false
	}
	if len(oid) != n {
		return false
	}
	for _, c := range oid {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func (e *Engine) criterion(ctx context.Context, r *model.Repository, c model.CompletionCriterion) observation {
	key := model.ID("criterion-", r.ID, c.Kind, c.ExpectedOID, c.ExpectedObjectFormat, strings.Join(c.Files, "\x00"), c.Literal)
	if obs, ok := e.cache[key]; ok {
		return obs
	}
	obs := e.observeCriterion(ctx, r, c)
	e.cache[key] = obs
	return obs
}
func (e *Engine) observeCriterion(ctx context.Context, r *model.Repository, c model.CompletionCriterion) observation {
	if !r.Complete || r.ChangedDuringScan {
		return observation{reason: "native repository coverage incomplete or changed"}
	}
	switch c.Kind {
	case "tree_represented":
		if c.ExpectedObjectFormat != r.ObjectFormat || !safeOID(c.ExpectedOID, r.ObjectFormat) {
			return observation{reason: "invalid tree ID/object format"}
		}
		for _, commit := range r.Commits {
			durable := false
			for _, class := range commit.Classes {
				if class == model.DurableRef || class == model.RemoteTracking {
					durable = true
				}
			}
			if durable && commit.TreeOID == c.ExpectedOID {
				return observation{known: true, satisfied: true, equivalent: true, evidence: []model.Evidence{fact(c.Kind, c.ExpectedOID, commit.OID)}}
			}
		}
		return observation{known: true, reason: "no matching tree in scanned normal-ref history"}
	case "commit_reachable", "patch_represented":
		if c.ExpectedObjectFormat != r.ObjectFormat || !safeOID(c.ExpectedOID, r.ObjectFormat) {
			return observation{reason: "missing/mismatched object format or invalid object ID"}
		}
		var target *model.Commit
		for i := range r.Commits {
			commit := &r.Commits[i]
			if commit.OID == c.ExpectedOID {
				target = commit
				break
			}
		}
		if target == nil {
			return observation{reason: "candidate commit not captured by native scan"}
		}
		for _, commit := range r.Commits {
			durable := false
			for _, class := range commit.Classes {
				if class == model.DurableRef || class == model.RemoteTracking {
					durable = true
				}
			}
			if !durable {
				continue
			}
			same := commit.OID == target.OID
			equivalent := c.Kind == "patch_represented" && target.PatchID != "" && commit.PatchID == target.PatchID
			if same || equivalent {
				return observation{known: true, satisfied: true, equivalent: !same, evidence: []model.Evidence{fact(c.Kind, target.OID, commit.OID)}}
			}
		}
		return observation{known: true, reason: "no exact/patch witness in scanned normal refs"}
	case "blob_at_path", "literal_in_file":
		if len(c.Files) != 1 || c.Files[0] == "" || strings.ContainsAny(c.Files[0], "\x00") || filepath.IsAbs(c.Files[0]) || strings.Contains(c.Files[0], "\\") || strings.HasPrefix(filepath.Clean(c.Files[0]), "..") {
			return observation{reason: "criterion needs one repository-relative path"}
		}
		if c.Kind == "blob_at_path" && (c.ExpectedObjectFormat != r.ObjectFormat || !safeOID(c.ExpectedOID, r.ObjectFormat)) {
			return observation{reason: "invalid expected blob/object format"}
		}
		if c.Kind == "literal_in_file" && (c.Literal == "" || len(c.Literal) > 4096) {
			return observation{reason: "criterion needs a bounded exact literal"}
		}
		for _, ref := range r.Refs {
			if ref.Class != model.DurableRef && ref.Class != model.RemoteTracking {
				continue
			}
			if !safeOID(ref.CommitOID, r.ObjectFormat) {
				continue
			}
			z := e.Git.Run(ctx, r.Path, "ls-tree", "-z", ref.CommitOID, "--", c.Files[0])
			if z.Code != 0 {
				return observation{reason: gitbackend.Failure(z)}
			}
			for _, record := range bytes.Split(z.Stdout, []byte{0}) {
				fields, path, ok := strings.Cut(string(record), "\t")
				parts := strings.Fields(fields)
				if !ok || len(parts) != 3 || path != c.Files[0] || parts[1] != "blob" {
					continue
				}
				oid := parts[2]
				match := oid == c.ExpectedOID
				if c.Kind == "literal_in_file" {
					// Batch header validates type/size before interpreting bytes. Native runner bounds output.
					content := e.Git.Input(ctx, r.Path, strings.NewReader(oid+"\n"), "cat-file", "--batch")
					if content.Code != 0 {
						return observation{reason: gitbackend.Failure(content)}
					}
					header, body, ok := bytes.Cut(content.Stdout, []byte{'\n'})
					h := strings.Fields(string(header))
					if !ok || len(h) != 3 || h[1] != "blob" {
						return observation{reason: "missing blob content"}
					}
					size, err := strconv.Atoi(h[2])
					if err != nil || size < 0 || size > len(body) {
						return observation{reason: "invalid batch content size"}
					}
					match = bytes.Contains(body[:size], []byte(c.Literal))
				}
				if match {
					return observation{known: true, satisfied: true, evidence: []model.Evidence{fact(c.Kind, r.ID+":"+path, ref.Name+":"+oid)}}
				}
			}
		}
		return observation{known: true, reason: "predicate not observed at path on scanned normal-ref tips"}
	default:
		return observation{reason: "unsupported completion criterion: " + c.Kind}
	}
}
func relatedPaths(i model.Intent) []string {
	paths := append([]string{}, i.Files...)
	for _, group := range i.Completion {
		for _, c := range group.Criteria {
			paths = append(paths, c.Files...)
		}
	}
	return paths
}
func pathDirty(c *model.Checkout, paths []string) bool {
	for _, path := range paths {
		for _, v := range c.Modified {
			if v.Path == path || v.OriginalPath == path {
				return true
			}
		}
		for _, v := range c.Staged {
			if v.Path == path || v.OriginalPath == path {
				return true
			}
		}
		for _, v := range c.Index {
			if v.Path == path {
				return true
			}
		}
		for _, p := range append(append([]string{}, c.Untracked...), c.Conflicted...) {
			if p == path {
				return true
			}
		}
	}
	return false
}
func (e *Engine) continuations(original model.Session, all []model.Session) []model.Session {
	out := []model.Session{}
	seen := map[string]bool{original.ID: true}
	queue := []model.Session{original}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range e.children[parent.ID] {
			if seen[child.ID] || child.HostID != original.HostID || child.StartedAt == nil || parent.EndedAt == nil || !child.StartedAt.After(*parent.EndedAt) {
				continue
			}
			linked := false
			for _, id := range child.Continues {
				if id == parent.ID {
					linked = true
				}
			}
			if !linked {
				continue
			}
			_, a := e.associate(original)
			_, b := e.associate(child)
			if a == nil || b == nil || a.Path != b.Path {
				continue
			}
			seen[child.ID] = true
			out = append(out, child)
			queue = append(queue, child)
		}
	}
	return out
}
func laterCommand(s model.Session, later []model.Session, i model.Intent, c model.CompletionCriterion) observation {
	if len(c.Commands) != 1 || s.EndedAt == nil {
		return observation{reason: "command criterion needs one exact command and ended session"}
	}
	for _, child := range later {
		for _, cmd := range child.Commands {
			if cmd.ExitCode == nil || *cmd.ExitCode != 0 || cmd.Interrupted || cmd.EndedAt == nil || !cmd.EndedAt.After(*s.EndedAt) || !filepath.IsAbs(cmd.WorkingDir) || cmd.EndedAt.After(time.Now().UTC()) || canonical(cmd.WorkingDir) != canonical(s.WorkingDir) || len(cmd.Evidence) == 0 {
				continue
			}
			bound := false
			for _, ref := range cmd.IntentRefs {
				if ref.SessionID == s.ID && ref.IntentID == i.ID {
					bound = true
				}
			}
			if bound && cmd.Command == c.Commands[0] {
				return observation{known: true, satisfied: true, evidence: []model.Evidence{{ID: model.ID("session-evidence-", child.ID, cmd.ID), Provider: child.Provider, Operation: "observed-command", Fact: "later linked command passed; does not prove broader goal", Subject: child.ID, Data: map[string]any{"command_id": cmd.ID, "evidence": cmd.Evidence}}}}
			}
		}
	}
	return observation{known: true, reason: "no intent-bound passing command in explicit later continuation"}
}
func intentContinued(s model.Session, i model.Intent, later []model.Session) bool {
	for _, child := range later {
		for _, intent := range child.Intents {
			for _, ref := range intent.Continues {
				if ref.SessionID == s.ID && ref.IntentID == i.ID {
					return true
				}
			}
		}
		for _, cmd := range child.Commands {
			for _, ref := range cmd.IntentRefs {
				if ref.SessionID == s.ID && ref.IntentID == i.ID {
					return true
				}
			}
		}
	}
	return false
}
func failed(s model.Session, i model.Intent) bool {
	for n := len(s.Commands) - 1; n >= 0; n-- {
		cmd := s.Commands[n]
		bound := false
		for _, id := range cmd.IntentIDs {
			if id == i.ID {
				bound = true
			}
		}
		if bound && len(cmd.Evidence) > 0 {
			return cmd.Interrupted || cmd.ExitCode != nil && *cmd.ExitCode != 0
		}
	}
	return false
}
func summary(intents []model.IntentAssessment) model.SessionSummary {
	if len(intents) == 0 {
		return model.SessionUnknown
	}
	resolved, unknown := 0, 0
	for _, i := range intents {
		if i.State == model.IntentCompleted || i.State == model.IntentSuperseded {
			resolved++
		}
		if i.State == model.IntentUnknown {
			unknown++
		}
	}
	if resolved == len(intents) {
		return model.SessionCompleted
	}
	if resolved > 0 {
		return model.SessionPartiallyCompleted
	}
	if unknown == len(intents) {
		return model.SessionUnknown
	}
	if unknown > 0 {
		return model.SessionMixed
	}
	return model.SessionUnresolved
}
func (e *Engine) Correlate(ctx context.Context, ss []model.Session) Result {
	result := Result{NativeObservation: e.Native.Scan, RelatedCheckouts: []model.Checkout{}, Format: "spuro-sessions", RelatedNativeFindings: []model.Finding{}, SchemaVersion: 1, Version: model.Version, HostID: e.HostID, ObservedAt: time.Now().UTC(), Sessions: ss, Native: e.Native, Sources: []SourceArtifact{}, Assessments: []model.DerivedSessionAssessment{}, Findings: []model.Finding{}, Evidence: []model.Evidence{}, Diagnostics: []model.Diagnostic{}}
	e.cache = map[string]observation{}
	e.children = map[string][]model.Session{}
	for _, s := range ss {
		for _, id := range s.Continues {
			e.children[id] = append(e.children[id], s)
		}
	}
	evidenceSeen := map[string]bool{}
	nativeFindingSeen := map[string]bool{}
	checkoutSeen := map[string]bool{}
	result.Diagnostics = append(result.Diagnostics, e.Native.Diagnostics...)
	for si := range result.Sessions {
		if err := ctx.Err(); err != nil {
			result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Provider: "sessions", Operation: "correlation", Message: err.Error(), Incomplete: true})
			break
		}
		s := &result.Sessions[si]
		r, c := e.associate(*s)
		s.Associations = nil
		if r != nil {
			key := r.ID + "\x00" + c.Path
			if !checkoutSeen[key] {
				result.RelatedCheckouts = append(result.RelatedCheckouts, *c)
				checkoutSeen[key] = true
			}
			s.Associations = append(s.Associations, model.SessionAssociation{HostID: e.HostID, RepoID: r.ID, Checkout: c.Path, CommonGitDir: r.CommonGitDir, Branch: c.Branch, Evidence: []string{"native canonical checkout containment"}})
		}
		coverage := normalizeCoverage(s.Coverage)
		coverage.GitHistory = model.CoverageObservation{Level: model.CoveragePartial, Scope: "currently scanned local refs", Reasons: []string{"later activity on other hosts, excluded checkouts, and fetched refs is not established"}}
		coverage.WorkingTree = model.CoverageObservation{Level: model.CoverageUnavailable, Reasons: []string{"no native host/path association"}}
		if c != nil && c.StatusKnown && r.Complete && !r.ChangedDuringScan {
			coverage.WorkingTree = model.CoverageObservation{Level: model.CoverageComplete, Scope: c.Path, Reasons: []string{"scoped native status observation; not whole-filesystem coverage"}}
		}
		coverage.SessionHistory = model.CoverageObservation{Level: model.CoveragePartial, Scope: "supplied session artifacts", Reasons: []string{"later sessions can be missing; no abandoned claim"}}
		s.Coverage = coverage
		for _, src := range s.Sources {
			id := model.ID("session-source-", src.Provider, src.Path, src.Modified.Format(time.RFC3339Nano))
			s.Evidence = append(s.Evidence, id)
			if !evidenceSeen[id] {
				result.Evidence = append(result.Evidence, model.Evidence{ID: id, Provider: src.Provider, ProviderVersion: src.ProducerVersion, Operation: "existing-artifact", Fact: "partial/stale artifact read without refresh", Subject: src.Path, Data: map[string]any{"format_version": src.FormatVersion, "sha256": src.SHA256, "modified": src.Modified, "earliest": src.Earliest, "latest": src.Latest, "retention_gaps": src.RetentionGaps}})
				evidenceSeen[id] = true
			}
		}
		later := e.continuations(*s, result.Sessions)
		assessment := model.DerivedSessionAssessment{SessionID: s.ID, Coverage: coverage, Intents: []model.IntentAssessment{}, Evidence: []string{}}
		for _, i := range s.Intents {
			a := model.IntentAssessment{IntentID: i.ID, State: model.IntentUnknown, Coverage: coverage, Confidence: model.Low, SatisfiedCriteria: []string{}, UnresolvedCriteria: []string{}, Evidence: append([]string{}, s.Evidence...), RelatedFindings: []string{}, Reasons: []string{}}
			all := len(i.Completion) > 0
			known := false
			equivalent := false
			for _, group := range i.Completion {
				groupOK := group.Mode == "ALL"
				for _, criterion := range group.Criteria {
					var obs observation
					if r == nil {
						obs.reason = "no verified host/checkout association"
					} else if criterion.Kind == "command_passed" {
						obs = laterCommand(*s, later, i, criterion)
					} else {
						obs = e.criterion(ctx, r, criterion)
					}
					known = known || obs.known
					equivalent = equivalent || obs.equivalent
					if obs.satisfied {
						a.SatisfiedCriteria = append(a.SatisfiedCriteria, criterion.ID)
					} else {
						a.UnresolvedCriteria = append(a.UnresolvedCriteria, criterion.ID)
						a.Reasons = append(a.Reasons, obs.reason)
					}
					if group.Mode == "ALL" {
						groupOK = groupOK && obs.satisfied
					} else {
						groupOK = groupOK || obs.satisfied
					}
					for _, v := range obs.evidence {
						a.Evidence = append(a.Evidence, v.ID)
						if !evidenceSeen[v.ID] {
							result.Evidence = append(result.Evidence, v)
							evidenceSeen[v.ID] = true
						}
					}
				}
				all = all && groupOK
			}
			cancelled := false
			for _, child := range later {
				for _, d := range child.Decisions {
					if d.SessionID == s.ID && d.IntentID == i.ID && d.Actor == "user" && d.Decision == "cancelled" && len(d.Evidence) > 0 {
						cancelled = true
						ev := model.Evidence{ID: model.ID("session-evidence-", "cancel", child.ID, i.ID), Provider: child.Provider, Operation: "user-decision", Fact: "explicit recorded user cancellation", Subject: i.ID, Data: map[string]any{"session_id": child.ID, "source_evidence": d.Evidence}}
						a.Evidence = append(a.Evidence, ev.ID)
						if !evidenceSeen[ev.ID] {
							result.Evidence = append(result.Evidence, ev)
							evidenceSeen[ev.ID] = true
						}
					}
				}
			}
			switch {
			case cancelled:
				a.State = model.IntentSuperseded
				a.Confidence = model.Medium
				a.Reasons = append(a.Reasons, "explicit user cancellation in a later linked session")
			case all:
				a.State = model.IntentCompleted
				a.Confidence = model.High
				for _, group := range i.Completion {
					for _, criterion := range group.Criteria {
						if criterion.Kind == "command_passed" {
							a.Confidence = model.Medium
						}
					}
				}
				if equivalent {
					a.State = model.IntentSuperseded
				}
				a.UnresolvedCriteria = nil
				a.Reasons = append(a.Reasons, "all supplied observable criteria met; broader semantic correctness not established")
			case c != nil && c.StatusKnown && r.Complete && !r.ChangedDuringScan && pathDirty(c, relatedPaths(i)):
				a.State = model.IntentDirtyStateRemains
				a.Confidence = model.High
				a.Reasons = append(a.Reasons, "associated path is dirty; session authorship is not established")
				ev := fact("status", c.Path, strings.Join(relatedPaths(i), ","))
				a.Evidence = append(a.Evidence, ev.ID)
				if !evidenceSeen[ev.ID] {
					result.Evidence = append(result.Evidence, ev)
					evidenceSeen[ev.ID] = true
				}
			case r != nil && intentContinued(*s, i, later):
				a.State = model.IntentResumedLater
				a.Confidence = model.Medium
				a.Reasons = append(a.Reasons, "explicit later session link; intent completion unproven")
			case r != nil && s.EndedAt != nil && failed(*s, i):
				a.State = model.IntentFailedUnresolved
				a.Confidence = model.High
				a.Reasons = append(a.Reasons, "intent-bound terminal command failed; later coverage partial")
				for n := len(s.Commands) - 1; n >= 0; n-- {
					cmd := s.Commands[n]
					bound := false
					for _, id := range cmd.IntentIDs {
						if id == i.ID {
							bound = true
						}
					}
					if !bound || len(cmd.Evidence) == 0 {
						continue
					}
					ev := model.Evidence{ID: model.ID("session-evidence-", "failed", s.ID, cmd.ID), Provider: s.Provider, Operation: "recorded-command", Fact: "recorded failure or interruption; command not rerun", Subject: s.ID, Data: map[string]any{"command_id": cmd.ID, "exit_code": cmd.ExitCode, "interrupted": cmd.Interrupted, "source_evidence": cmd.Evidence}}
					a.Evidence = append(a.Evidence, ev.ID)
					if !evidenceSeen[ev.ID] {
						result.Evidence = append(result.Evidence, ev)
						evidenceSeen[ev.ID] = true
					}
					break
				}

			case r != nil && s.EndedAt != nil && i.Explicit && len(i.Completion) == 0:
				a.State = model.IntentPromisedNotObserved
				a.Confidence = model.Medium
				a.Reasons = append(a.Reasons, "candidate intent lacks concrete completion criteria; not an abandonment claim")
			case r != nil && known:
				a.State = model.IntentUnresolved
				a.Confidence = model.Medium
			default:
				a.Reasons = append(a.Reasons, "insufficient association or supported criteria")
			}
			for _, f := range e.Native.Findings {
				if c != nil && f.Checkout == c.Path {
					a.RelatedFindings = append(a.RelatedFindings, f.ID)
					if !nativeFindingSeen[f.ID] {
						result.RelatedNativeFindings = append(result.RelatedNativeFindings, f)
						nativeFindingSeen[f.ID] = true
					}
				}
			}
			assessment.Intents = append(assessment.Intents, a)
			severity := model.Review
			if a.State == model.IntentCompleted || a.State == model.IntentSuperseded || a.State == model.IntentResumedLater {
				severity = model.Info
			}
			// Session metadata cannot elevate severity above matching native preservation evidence.
			if a.State == model.IntentDirtyStateRemains {
				for _, f := range e.Native.Findings {
					if f.Checkout != c.Path || f.Severity != model.PreserveFirst {
						continue
					}
					for _, p := range relatedPaths(i) {
						for _, fp := range f.Paths {
							if p == fp {
								severity = model.PreserveFirst
							}
						}
					}
				}
			}
			repoID, checkout := "", ""
			if r != nil {
				repoID = r.ID
				checkout = c.Path
			}
			result.Findings = append(result.Findings, model.Finding{ID: model.ID("session-finding-", s.HostID, s.ID, i.ID), Severity: severity, Kind: "session_" + strings.ToLower(string(a.State)), RepoID: repoID, Checkout: checkout, Summary: fmt.Sprintf("%s: %s", a.State, Redact(i.Description)), Evidence: a.Evidence, Confidence: a.Confidence, Paths: relatedPaths(i), SuggestedAction: "Review scoped evidence and current state; no automatic resumption or cleanup."})
		}
		assessment.Summary = summary(assessment.Intents)
		result.Assessments = append(result.Assessments, assessment)
	}
	neededNativeEvidence := map[string]bool{}
	for _, f := range result.RelatedNativeFindings {
		for _, id := range f.Evidence {
			neededNativeEvidence[id] = true
		}
	}
	for _, r := range e.Native.Repositories {
		for _, ev := range r.Evidence {
			if neededNativeEvidence[ev.ID] && !evidenceSeen[ev.ID] {
				result.Evidence = append(result.Evidence, ev)
				evidenceSeen[ev.ID] = true
			}
		}
	}

	sort.Slice(result.Findings, func(i, j int) bool { return result.Findings[i].ID < result.Findings[j].ID })
	return result
}
