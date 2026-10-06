package sessions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/MTG-Thomas/spuro/internal/model"
)

type contextRecord struct {
	request *model.BoundedRequest
	tracker *model.TrackerWitness
	revert  *model.ScopedRevert
	stash   *model.StashArtifact
}

func validProvenance(p model.ContextProvenance, at time.Time) bool {
	return p.Provider != "" && p.Artifact != "" && p.RecordID != "" && len(p.Evidence) > 0 && !p.ObservedAt.IsZero() && !at.IsZero() && !p.ObservedAt.Before(at)
}
func validTarget(t model.IntentRef) bool {
	return t.SessionID != "" && t.IntentID != "" && !strings.ContainsRune(t.SessionID+t.IntentID, 0)
}
func validContextPath(p string) bool {
	return p != "" && p != "." && p != ".." && !path.IsAbs(p) && path.Clean(p) == p && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\\\x00*?[")
}
func validSubject(s model.TrackerSubject) bool {
	return s.ForgeHost != "" && s.Repository != "" && s.Kind != "" && s.Number > 0 && s.Action != ""
}

// ValidateContext is shared by normalized artifacts and the offline reader.
func ValidateContext(c *model.ContextEvidence) error {
	if c == nil {
		return nil
	}
	if c.SchemaVersion != 1 {
		return fmt.Errorf("unsupported context evidence schema")
	}
	if len(c.BoundedRequests)+len(c.TrackerWitnesses)+len(c.ScopedReverts)+len(c.StashArtifacts) > 10000 {
		return fmt.Errorf("context observation limit exceeded")
	}
	ids := map[string]bool{}
	for _, r := range c.BoundedRequests {
		if r.ID == "" || ids[r.ID] || !validTarget(r.Target) || !validProvenance(r.Provenance, r.At) || r.Actor != "user" || r.HostID == "" || r.RepoID == "" || len(r.AllowedPaths) == 0 {
			return fmt.Errorf("invalid bounded request observation")
		}
		ids[r.ID] = true
		for _, p := range r.AllowedPaths {
			if !validContextPath(p) {
				return fmt.Errorf("bounded requests require exact relative Git paths")
			}
		}
	}
	for _, r := range c.TrackerWitnesses {
		if r.ID == "" || ids[r.ID] || !validTarget(r.Target) || !validProvenance(r.Provenance, r.EventAt) || !validSubject(r.Subject) || r.Outcome == "" || r.CheckedBy == "" {
			return fmt.Errorf("invalid checked tracker observation")
		}
		ids[r.ID] = true
	}
	for _, r := range c.ScopedReverts {
		if r.ID == "" || ids[r.ID] || !validTarget(r.Target) || r.Actor != "user" || r.HostID == "" || r.ThreadID == "" || !exactEvent(r.Event, r.Request, "user_message") || !revertRequest(r.Request) {
			return fmt.Errorf("invalid scoped revert request")
		}
		ids[r.ID] = true
	}
	for _, r := range c.StashArtifacts {
		if r.ID == "" || ids[r.ID] || !validTarget(r.Target) || r.HostID == "" || r.RepoID == "" || r.CommonGitDir == "" || r.ParentNumber != 3 || r.ObjectType != "blob" || !validContextPath(r.Path) || !validOID(r.StashOID, r.ObjectFormat) || !validOID(r.ParentOID, r.ObjectFormat) || !validOID(r.BlobOID, r.ObjectFormat) || !validProvenance(r.Provenance, r.CheckedAt) {
			return fmt.Errorf("invalid untracked stash artifact")
		}
		ids[r.ID] = true
	}
	return nil
}
func indexContext(ss []model.Session) map[string][]contextRecord {
	out := map[string][]contextRecord{}
	for _, s := range ss {
		c := s.Context
		if ValidateContext(c) != nil || c == nil {
			continue
		}
		for n := range c.BoundedRequests {
			r := &c.BoundedRequests[n]
			key := r.Target.SessionID + "\x00" + r.Target.IntentID
			out[key] = append(out[key], contextRecord{request: r})
		}
		for n := range c.ScopedReverts {
			r := &c.ScopedReverts[n]
			key := r.Target.SessionID + "\x00" + r.Target.IntentID
			out[key] = append(out[key], contextRecord{revert: r})
		}
		for n := range c.StashArtifacts {
			r := &c.StashArtifacts[n]
			key := r.Target.SessionID + "\x00" + r.Target.IntentID
			out[key] = append(out[key], contextRecord{stash: r})
		}
		for n := range c.TrackerWitnesses {
			r := &c.TrackerWitnesses[n]
			key := r.Target.SessionID + "\x00" + r.Target.IntentID
			out[key] = append(out[key], contextRecord{tracker: r})
		}
	}
	return out
}
func matchContext(records []contextRecord, s *model.Session, i *model.Intent) ([]IntentMatch, []string) {
	matches := []IntentMatch{}
	gaps := []string{}
	if len(records) == 0 {
		return matches, gaps
	}
	if !boundEvent(i) {
		return matches, []string{"context records need an actual source event, matching recorded_at, and exact description hash binding; artifact/schema or last-assistant timestamps are not original intent event time"}
	}
	for _, record := range records {
		if w := record.revert; w != nil {
			if w.HostID != s.HostID || w.ThreadID != threadIdentity(s) || !revertChronology(i, w) {
				gaps = append(gaps, "scoped revert does not match original host/thread or scoped chronology")
				continue
			}
			matches = append(matches, IntentMatch{Kind: "scoped_user_revert_request", RecordID: w.ID, Provenance: &w.Event.Provenance, Reason: "supplied exact user request scopes revert/archive to this thread; execution, cancellation and residual-diff authorship are not established", Evidence: w.Event.Provenance.Evidence})
			gaps = append(gaps, "keep unrelated residual dirty work as separate preservation risk; no historic durability or whole-repository completion proof")
		}
		if w := record.stash; w != nil {
			if w.HostID != s.HostID || w.RepoID != i.TargetRepoID || !resolvedTarget(i, s.HostID, w.RepoID) || !stashTarget(s, w) {
				gaps = append(gaps, "stash witness lacks exact host/repository/common-dir target binding")
				continue
			}
			for _, a := range i.Artifacts {
				if a.Path != w.Path {
					continue
				}
				if a.ExpectedBlobOID != "" && a.ExpectedBlobOID != w.BlobOID {
					gaps = append(gaps, "stash blob differs from requested content version")
					continue
				}
				matches = append(matches, IntentMatch{Kind: "SOURCE_AVAILABLE_IN_STASH", RecordID: w.ID, RepoID: w.RepoID, StashArtifact: w, Reason: "supplied checked blob exists at this exact stash third-parent path; availability only, not requested-version execution, policy assignment or durable backup", Evidence: w.Provenance.Evidence})
				gaps = append(gaps, "stash availability does not prove the requested later script version or intent execution")
			}
		}

		if r := record.request; r != nil {
			if !r.At.After(*i.RecordedAt) || r.HostID != s.HostID || i.TargetRepoID == "" || r.RepoID != i.TargetRepoID || !resolvedTarget(i, s.HostID, r.RepoID) {
				gaps = append(gaps, "bounded request chronology/explicit target differs or is missing; cwd is not substituted")
				continue
			}
			matches = append(matches, IntentMatch{Kind: "bounded_user_request_record", RecordID: r.ID, RepoID: r.RepoID, AllowedPaths: r.AllowedPaths, OtherWorkForbidden: r.OtherWorkForbidden, Provenance: &r.Provenance, Reason: "supplied later user request names these exact paths; restrictions are separate; no completion, cancellation, or current-action authorization proof", Evidence: append(append([]string{}, i.Evidence...), r.Provenance.Evidence...)})
			if i.Origin == "task_board" && r.OtherWorkForbidden {
				gaps = append(gaps, "broad task board must be reconciled against the later bounded user request; no whole-board supersession inferred")
			}
		}
		if r := record.tracker; r != nil {
			if i.Tracker == nil || *i.Tracker != r.Subject || !r.EventAt.After(*i.RecordedAt) {
				gaps = append(gaps, "tracker witness does not match exact intent subject or later chronology")
				continue
			}
			if i.Tracker.Kind != "pull_request" || i.Tracker.Action != "merge" || r.Outcome != "merged" {
				gaps = append(gaps, "tracker outcome does not substantiate this exact merge predicate; closure is not merge/deployment/durability proof")
				continue
			}
			matches = append(matches, IntentMatch{Kind: "supplied_checked_tracker_merge", RecordID: r.ID, Tracker: &r.Subject, Provenance: &r.Provenance, Reason: "producer's checked tracker record matches only this exact repository/PR merge intent; no other-intent, deployment, or durable-backup claim", Evidence: append(append([]string{}, i.Evidence...), r.Provenance.Evidence...)})
		}
	}
	return matches, gaps
}

func mergeContext(a, b *model.ContextEvidence) (*model.ContextEvidence, error) {
	if err := ValidateContext(a); err != nil {
		return nil, err
	}
	if err := ValidateContext(b); err != nil {
		return nil, err
	}
	if a == nil {
		return b, nil
	}
	if b == nil {
		return a, nil
	}
	out := &model.ContextEvidence{SchemaVersion: 1, BoundedRequests: append([]model.BoundedRequest{}, a.BoundedRequests...), TrackerWitnesses: append([]model.TrackerWitness{}, a.TrackerWitnesses...)}
	seen := map[string]string{}
	for _, r := range a.BoundedRequests {
		v, _ := json.Marshal(r)
		seen[r.ID] = string(v)
	}
	for _, r := range a.TrackerWitnesses {
		v, _ := json.Marshal(r)
		seen[r.ID] = string(v)
	}
	for _, r := range b.BoundedRequests {
		v, _ := json.Marshal(r)
		if old, ok := seen[r.ID]; ok {
			if old != string(v) {
				return nil, fmt.Errorf("conflicting context observation")
			}
		} else {
			out.BoundedRequests = append(out.BoundedRequests, r)
			seen[r.ID] = string(v)
		}
	}
	for _, r := range b.TrackerWitnesses {
		v, _ := json.Marshal(r)
		if old, ok := seen[r.ID]; ok {
			if old != string(v) {
				return nil, fmt.Errorf("conflicting context observation")
			}
		} else {
			out.TrackerWitnesses = append(out.TrackerWitnesses, r)
			seen[r.ID] = string(v)
		}
	}
	if err := mergeExtraContext(out, a, b); err != nil {
		return nil, err
	}
	return out, ValidateContext(out)
}

func boundEvent(i *model.Intent) bool {
	e := i.SourceEvent
	if e == nil || i.RecordedAt == nil || i.RecordedAt.IsZero() || !i.RecordedAt.Equal(e.At) || len(i.Evidence) == 0 || e.Match != "exact_payload" || !validProvenance(e.Provenance, e.At) {
		return false
	}
	switch e.Kind {
	case "todo_write", "user_message", "assistant_message", "tool_result":
	default:
		return false
	}
	if i.Origin == "task_board" && e.Kind != "todo_write" {
		return false
	}
	if i.Origin == "user_request" && e.Kind != "user_message" {
		return false
	}
	if i.Description == "" {
		return false
	}
	sum := sha256.Sum256([]byte(i.Description))
	return e.DescriptionSHA256 == hex.EncodeToString(sum[:])
}
func resolvedTarget(i *model.Intent, host, repo string) bool {
	t := i.TargetResolution
	return t != nil && t.HostID == host && t.RepoID == repo && (t.Method == "explicit_repository" || t.Method == "verified_user_path") && validProvenance(t.Provenance, *i.RecordedAt)
}

func exactEvent(e model.IntentEvent, payload, kind string) bool {
	sum := sha256.Sum256([]byte(payload))
	return payload != "" && e.Kind == kind && e.Match == "exact_payload" && e.DescriptionSHA256 == hex.EncodeToString(sum[:]) && validProvenance(e.Provenance, e.At)
}
func revertRequest(s string) bool {
	return strings.TrimSuffix(strings.TrimSpace(s), ".") == "Revert the changes from this thread and archive"
}
func validOID(oid, format string) bool {
	size := 40
	if format == "sha256" {
		size = 64
	} else if format != "sha1" {
		return false
	}
	if len(oid) != size || strings.ToLower(oid) != oid {
		return false
	}
	_, err := hex.DecodeString(oid)
	return err == nil
}
func stashTarget(s *model.Session, w *model.StashArtifact) bool {
	for _, a := range s.Associations {
		if a.HostID == w.HostID && a.RepoID == w.RepoID && a.CommonGitDir == w.CommonGitDir {
			return true
		}
	}
	return false
}
func mergeExtraContext(out, a, b *model.ContextEvidence) error {
	seen := map[string]string{}
	add := func(id string, v any) (bool, error) {
		data, _ := json.Marshal(v)
		if old, ok := seen[id]; ok {
			if old != string(data) {
				return false, fmt.Errorf("conflicting context observation")
			}
			return false, nil
		}
		seen[id] = string(data)
		return true, nil
	}
	for _, c := range []*model.ContextEvidence{a, b} {
		for _, v := range c.BoundedRequests {
			if _, e := add(v.ID, v); e != nil {
				return e
			}
		}
		for _, v := range c.TrackerWitnesses {
			if _, e := add(v.ID, v); e != nil {
				return e
			}
		}
		for _, v := range c.ScopedReverts {
			yes, e := add(v.ID, v)
			if e != nil {
				return e
			}
			if yes {
				out.ScopedReverts = append(out.ScopedReverts, v)
			}
		}
		for _, v := range c.StashArtifacts {
			yes, e := add(v.ID, v)
			if e != nil {
				return e
			}
			if yes {
				out.StashArtifacts = append(out.StashArtifacts, v)
			}
		}
	}
	return nil
}

func revertChronology(i *model.Intent, w *model.ScopedRevert) bool {
	if residualObservation(i) {
		return w.Event.At.Before(*i.RecordedAt)
	}
	return w.Event.At.After(*i.RecordedAt)
}
