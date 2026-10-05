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
	if len(c.BoundedRequests)+len(c.TrackerWitnesses) > 10000 {
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
