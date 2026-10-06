package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/MTG-Thomas/spuro/internal/model"
)

// UnknownQueue adds inspection work; it never changes a lifecycle assessment.
type UnknownQueue struct {
	Groups      []UnknownGroup `json:"groups"`
	Sightings   int            `json:"sightings"`
	Limitations []string       `json:"limitations"`
}
type UnknownGroup struct {
	ObservationKind    string            `json:"observation_kind,omitempty"`
	ID                 string            `json:"id"`
	HostID             string            `json:"host_id"`
	ThreadID           string            `json:"thread_id"`
	Description        string            `json:"description,omitempty"`
	Members            []model.IntentRef `json:"members"`
	Gaps               []string          `json:"gaps"`
	Matches            []IntentMatch     `json:"deterministic_matches"`
	Suggestions        []IntentMatch     `json:"fuzzy_suggestions"`
	IdentityCandidates []IntentMatch     `json:"identity_candidates"`
	Priority           string            `json:"inspection_priority"`
	NextStep           string            `json:"next_step"`
}
type IntentMatch struct {
	StashArtifact      *model.StashArtifact     `json:"stash_artifact,omitempty"`
	OtherWorkForbidden bool                     `json:"other_work_forbidden,omitempty"`
	RecordID           string                   `json:"record_id,omitempty"`
	Provenance         *model.ContextProvenance `json:"provenance,omitempty"`
	AllowedPaths       []string                 `json:"allowed_paths,omitempty"`
	Tracker            *model.TrackerSubject    `json:"tracker_subject,omitempty"`
	ObservedState      model.IntentState        `json:"observed_intent_state,omitempty"`
	Kind               string                   `json:"kind"`
	SessionID          string                   `json:"session_id,omitempty"`
	IntentID           string                   `json:"intent_id,omitempty"`
	RepoID             string                   `json:"repo_id,omitempty"`
	CriterionID        string                   `json:"criterion_id,omitempty"`
	OID                string                   `json:"oid,omitempty"`
	Reason             string                   `json:"reason"`
	Evidence           []string                 `json:"evidence"`
}
type intentRecord struct {
	session    *model.Session
	intent     *model.Intent
	assessment *model.IntentAssessment
}

func timestamp(s *model.Session) *time.Time {
	if s.EndedAt != nil {
		return s.EndedAt
	}
	return s.LastObservedAt
}
func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }) {
		if len(w) < 3 {
			continue
		}
		switch w {
		case "next", "todo", "remaining", "need", "still", "will", "then", "the", "and", "for", "with", "this", "that", "add", "update", "fix", "tests", "test", "docs":
			continue
		}
		out[w] = true
	}
	return out
}
func overlap(a, b map[string]bool) bool {
	shared := 0
	for w := range a {
		if b[w] {
			shared++
		}
	}
	return shared >= 3 && float64(shared)/float64(len(a)+len(b)-shared) >= 0.5
}
func scope(s *model.Session) string {
	if s.HostID == "" {
		return ""
	}
	repos := map[string]bool{}
	for _, a := range s.Associations {
		if a.HostID == s.HostID && a.RepoID != "" {
			repos[a.RepoID] = true
		}
	}
	if len(repos) != 1 {
		return ""
	}
	for id := range repos {
		return s.HostID + "\x00" + id
	}
	return ""
}
func sameTarget(ref model.IntentRef, sID, iID string) bool {
	return ref.SessionID == sID && ref.IntentID == iID
}
func durable(c model.Commit) bool {
	for _, v := range c.Classes {
		if v == model.DurableRef || v == model.RemoteTracking {
			return true
		}
	}
	return false
}

// FindUnknowns uses only the supplied snapshot. No filesystem/Git/provider reads.
func FindUnknowns(ctx context.Context, r *Result) UnknownQueue {
	q := UnknownQueue{Groups: []UnknownGroup{}, Limitations: []string{"cached evidence is scoped to the original scan, not current state or a verified backup", "cwd association is context; intent target and semantic completion require review", "fuzzy matches suggest inspection only; later artifact timestamps do not prove session continuation"}}
	records := []intentRecord{}
	assessments := map[string]*model.IntentAssessment{}
	for si := range r.Assessments {
		s := &r.Assessments[si]
		for ai := range s.Intents {
			a := &s.Intents[ai]
			assessments[s.SessionID+"\x00"+a.IntentID] = a
		}
	}
	for si := range r.Sessions {
		s := &r.Sessions[si]
		for ii := range s.Intents {
			i := &s.Intents[ii]
			records = append(records, intentRecord{s, i, assessments[s.ID+"\x00"+i.ID]})
		}
	}
	groups := map[string]int{}
	contexts := indexContext(r.Sessions)
	linked := map[string][]*model.Session{}
	for n := range r.Sessions {
		later := &r.Sessions[n]
		keys := map[string]bool{}
		for _, d := range later.Decisions {
			keys[d.SessionID+"\x00"+d.IntentID] = true
		}
		for _, i := range later.Intents {
			for _, ref := range i.Continues {
				keys[ref.SessionID+"\x00"+ref.IntentID] = true
			}
		}
		for key := range keys {
			linked[key] = append(linked[key], later)
		}
	}
	// Index discriminating tokens within host/repository context; avoid whole-corpus pair scans.
	postings := map[string][]int{}
	identities := map[string][]int{}
	for n, rec := range records {
		if observationOnly(rec.intent) {
			continue
		}
		if rec.session.HostID != "" && strings.TrimSpace(rec.intent.Description) != "" {
			identities[descriptionIdentity(rec.session, rec.intent)] = append(identities[descriptionIdentity(rec.session, rec.intent)], n)
		}
		sc := scope(rec.session)
		if sc == "" {
			continue
		}
		for word := range tokens(rec.intent.Description) {
			key := sc + "\x00" + word
			postings[key] = append(postings[key], n)
		}
	}
	for _, rec := range records {
		if ctx.Err() != nil {
			q.Limitations = append(q.Limitations, "reconciliation interrupted; queue incomplete")
			break
		}
		if rec.assessment == nil || (rec.assessment.State != model.IntentUnknown && rec.assessment.State != model.IntentPromisedNotObserved) {
			continue
		}
		q.Sightings++
		s, i := rec.session, rec.intent
		thread := threadIdentity(s)
		key := unknownGroupKey(s, i)
		member := model.IntentRef{SessionID: s.ID, IntentID: i.ID}
		g := UnknownGroup{ID: key, HostID: s.HostID, ThreadID: thread, Description: i.Description, Members: []model.IntentRef{member}, IdentityCandidates: []IntentMatch{}, Gaps: []string{}, Matches: []IntentMatch{}, Suggestions: []IntentMatch{}, Priority: "BACKGROUND", NextStep: "clarify the bounded user requirement and attach observable criteria"}
		if controlOnly(i) {
			g.ObservationKind = "control_boilerplate_candidate"
			if typedControl(i) {
				g.ObservationKind = "harness_control_event"
			}
			g.Gaps = append(g.Gaps, "control-only event is not an independent human request or later activity; prior substantive work remains unresolved")
		}
		lower := strings.ToLower(i.Description)
		for _, marker := range []string{"optional", "if needed", "maybe", "consider", "task board"} {
			if strings.Contains(lower, marker) {
				g.Gaps = append(g.Gaps, "requirement may be optional or broader than the bounded user request; inspect original scope")
				break
			}
		}
		sc := scope(s)
		if sc == "" {
			g.Gaps = append(g.Gaps, "missing or ambiguous host/repository association")
		} else {
			g.Gaps = append(g.Gaps, "repository association is contextual; intended target is not established")
		}
		if len(i.Completion) == 0 {
			g.Gaps = append(g.Gaps, "no observable completion criteria")
		}
		if timestamp(s) == nil {
			g.Gaps = append(g.Gaps, "no chronological anchor")
		}
		if rec.assessment.Coverage.SessionHistory.Level != model.CoverageComplete || rec.assessment.Coverage.GitHistory.Level != model.CoverageComplete {
			g.Gaps = append(g.Gaps, "later session/Git coverage incomplete")
		}
		for _, id := range rec.assessment.SatisfiedCriteria {
			g.Matches = append(g.Matches, IntentMatch{Kind: "already_observed_criterion", CriterionID: id, Reason: "original assessment contains this satisfied predicate; broader intent remains unresolved", Evidence: rec.assessment.Evidence})
		}
		contextMatches, contextGaps := matchContext(contexts[s.ID+"\x00"+i.ID], s, i)
		g.Matches = append(g.Matches, contextMatches...)
		g.Gaps = append(g.Gaps, contextGaps...)
		// Exact links are not guessed from thread names or prose.
		for _, later := range linked[s.ID+"\x00"+i.ID] {
			if later.HostID != s.HostID || s.HostID == "" || timestamp(s) == nil || timestamp(later) == nil || !timestamp(later).After(*timestamp(s)) {
				continue
			}
			for _, decision := range later.Decisions {
				if decision.SessionID == s.ID && decision.IntentID == i.ID && decision.Actor == "user" && decision.Decision == "cancel" && len(decision.Evidence) > 0 {
					g.Matches = append(g.Matches, IntentMatch{Kind: "explicit_user_cancellation_record", SessionID: later.ID, IntentID: i.ID, Reason: "later user decision targets this exact intent; verify source and scope", Evidence: decision.Evidence})
				}
			}
			for _, li := range later.Intents {
				for _, link := range li.Continues {
					if sameTarget(link, s.ID, i.ID) {
						g.Matches = append(g.Matches, IntentMatch{Kind: "explicit_intent_continuation", SessionID: later.ID, IntentID: li.ID, Reason: "qualified continuation link; completion not implied", Evidence: li.Evidence})
					}
				}
			}
		}
		if r.Native != nil && sc != "" && r.HostID != "" && s.HostID == r.HostID {
			for _, repo := range r.Native.Repositories {
				if s.HostID+"\x00"+repo.ID != sc || !repo.Complete || repo.ChangedDuringScan {
					continue
				}
				for _, cg := range i.Completion {
					for _, criterion := range cg.Criteria {
						if criterion.Kind != "commit_reachable" && criterion.Kind != "tree_represented" && criterion.Kind != "patch_represented" {
							continue
						}
						if !safeOID(criterion.ExpectedOID, criterion.ExpectedObjectFormat) || criterion.ExpectedObjectFormat != repo.ObjectFormat {
							continue
						}
						targetPatch := ""
						for _, commit := range repo.Commits {
							if commit.OID == criterion.ExpectedOID {
								targetPatch = commit.PatchID
							}
						}
						for _, commit := range repo.Commits {
							if !durable(commit) {
								continue
							}
							match := false
							switch criterion.Kind {
							case "commit_reachable":
								match = commit.OID == criterion.ExpectedOID
							case "tree_represented":
								match = commit.TreeOID == criterion.ExpectedOID
							case "patch_represented":
								match = targetPatch != "" && commit.PatchID == targetPatch
							}
							if match {
								g.Matches = append(g.Matches, IntentMatch{Kind: "cached_" + criterion.Kind, RepoID: repo.ID, CriterionID: criterion.ID, OID: commit.OID, Reason: "narrow predicate matches a normal-ref commit in the supplied native snapshot; scope/semantic completion not established", Evidence: criterion.Evidence})
								break
							}
						}
					}
				}
			}
		}
		for _, n := range identities[descriptionIdentity(s, i)] {
			other := records[n]
			if other.session.ID == s.ID || unknownGroupKey(other.session, other.intent) == key {
				continue
			}
			if len(g.IdentityCandidates) == 3 {
				g.Gaps = append(g.Gaps, "additional identity candidates omitted after three references")
				break
			}
			g.IdentityCandidates = append(g.IdentityCandidates, IntentMatch{Kind: "same_thread_description_sighting", SessionID: other.session.ID, IntentID: other.intent.ID, Reason: "same host-qualified provider thread and exact description; context/kind/payload may differ; reconcile identity, not independent later activity", Evidence: other.intent.Evidence})
		}
		// Fuzzy candidates are bounded and sorted by input order, never lifecycle proof.
		candidates := map[int]bool{}
		words := tokens(i.Description)
		if sc != "" && timestamp(s) != nil {
			for word := range words {
				for _, n := range postings[sc+"\x00"+word] {
					candidates[n] = true
				}
			}
		}
		indexes := []int{}
		for n := range candidates {
			indexes = append(indexes, n)
		}
		sort.Ints(indexes)
		for _, n := range indexes {
			other := records[n]
			t := timestamp(other.session)
			if observationOnly(i) || observationOnly(other.intent) || other.session.ID == s.ID || unknownGroupKey(other.session, other.intent) == key || descriptionIdentity(other.session, other.intent) == descriptionIdentity(s, i) || t == nil || !t.After(*timestamp(s)) || !overlap(words, tokens(other.intent.Description)) {
				continue
			}
			if len(g.Suggestions) >= 3 {
				g.Gaps = append(g.Gaps, "additional fuzzy matches omitted after three suggestions")
				break
			}
			state := model.IntentUnknown
			if other.assessment != nil {
				state = other.assessment.State
			}
			g.Suggestions = append(g.Suggestions, IntentMatch{Kind: "similar_later_intent", ObservedState: state, SessionID: other.session.ID, IntentID: other.intent.ID, Reason: "at least three discriminating tokens and >=0.5 token overlap in the same contextual host/repository; inspect bounded request and later evidence", Evidence: other.intent.Evidence})
		}
		if len(g.Matches) > 0 {
			g.Priority = "EVIDENCE_READY"
			g.NextStep = "inspect exact witnesses and target scope; reconcile each concrete criterion"
		} else if len(g.Suggestions) > 0 {
			g.Priority = "FOLLOW_UP_CANDIDATE"
			g.NextStep = "inspect suggested later intents for completion, cancellation, or revised scope"
		} else if len(g.IdentityCandidates) > 0 {
			g.Priority = "CONCRETE_CANDIDATE"
			g.NextStep = "reconcile schema/context identity before treating sightings as later activity"
		} else if len(i.Files) > 0 || len(i.Artifacts) > 0 || len(i.Completion) > 0 {
			g.Priority = "CONCRETE_CANDIDATE"
			g.NextStep = "resolve intended repository, then inspect supplied paths and missing criteria"
		}
		for _, m := range contextMatches {
			if m.Kind == "bounded_user_request_record" {
				if m.OtherWorkForbidden {
					g.NextStep = "inspect bounded user request and authorized paths; do not resume the whole task board"
				} else {
					g.NextStep = "inspect the user request before inferring whole-board authorization or supersession"
				}
				break
			}
		}
		if residualObservation(i) {
			g.ObservationKind = "residual_dirty_observation"
			g.Gaps = append(g.Gaps, "assistant disavow is not authored unfinished intent; retain independent dirty-state risk and uncertain historical authorship")
		}
		if observationOnly(i) {
			g.Priority = "BACKGROUND"
			g.NextStep = "inspect producer event typing; keep the interrupted substantive request unresolved"
			if residualObservation(i) {
				if len(g.Matches) > 0 {
					g.Priority = "EVIDENCE_READY"
				}
				g.NextStep = "inspect separate dirty preservation findings; no authorship or historical durability inferred"
			}
			g.Suggestions = nil
			g.IdentityCandidates = nil
		}
		for _, m := range contextMatches {
			if m.Kind == "scoped_user_revert_request" {
				g.NextStep = "review exact thread-scoped user instruction separately from residual dirty preservation risk"
			}
		}
		if n, ok := groups[key]; ok {
			q.Groups[n].Members = append(q.Groups[n].Members, member)
			q.Groups[n].Matches = append(q.Groups[n].Matches, g.Matches...)
			q.Groups[n].Suggestions = append(q.Groups[n].Suggestions, g.Suggestions...)
			q.Groups[n].IdentityCandidates = append(q.Groups[n].IdentityCandidates, g.IdentityCandidates...)
			q.Groups[n].Gaps = append(q.Groups[n].Gaps, g.Gaps...)
			if priorityRank(g.Priority) < priorityRank(q.Groups[n].Priority) {
				q.Groups[n].Priority = g.Priority
				q.Groups[n].NextStep = g.NextStep
			}
		} else {
			groups[key] = len(q.Groups)
			q.Groups = append(q.Groups, g)
		}
	}

	for n := range q.Groups {
		g := &q.Groups[n]
		g.Matches = uniqueMatches(g.Matches)
		g.Suggestions = uniqueMatches(g.Suggestions)
		g.IdentityCandidates = uniqueMatches(g.IdentityCandidates)
		if len(g.IdentityCandidates) > 3 {
			g.IdentityCandidates = g.IdentityCandidates[:3]
		}
		if len(g.Suggestions) > 3 {
			g.Suggestions = g.Suggestions[:3]
			g.Gaps = append(g.Gaps, "additional fuzzy matches omitted after three suggestions")
		}
		seen := map[string]bool{}
		gaps := []string{}
		for _, gap := range g.Gaps {
			if !seen[gap] {
				gaps = append(gaps, gap)
				seen[gap] = true
			}
		}
		g.Gaps = gaps
	}
	sort.Slice(q.Groups, func(i, j int) bool {
		a, b := q.Groups[i], q.Groups[j]
		if priorityRank(a.Priority) != priorityRank(b.Priority) {
			return priorityRank(a.Priority) < priorityRank(b.Priority)
		}
		return a.ID < b.ID
	})
	return q
}

func UnknownText(w interface{ Write([]byte) (int, error) }, q UnknownQueue) error {
	if _, err := fmt.Fprintf(w, "UNKNOWN reconciliation: %d sightings; %d exact payload groups\n", q.Sightings, len(q.Groups)); err != nil {
		return err
	}
	shown := 0
	actionable := false
	for _, g := range q.Groups {
		if g.Priority != "BACKGROUND" {
			actionable = true
			break
		}
	}
	for _, g := range q.Groups {
		if g.Priority == "BACKGROUND" && (actionable || shown >= 5) {
			continue
		}
		if shown == 20 {
			break
		}
		shown++
		fmt.Fprintf(w, "%s %s (%d sightings)\n  %s\n", g.Priority, g.ID, len(g.Members), g.Description)
		if len(g.Members) > 0 {
			fmt.Fprintf(w, "  host %q; thread %q; inspect %s / %s\n", g.HostID, g.ThreadID, g.Members[0].SessionID, g.Members[0].IntentID)
		}
		for _, m := range g.Matches {
			if len(m.AllowedPaths) > 0 {
				fmt.Fprintf(w, "  bounded paths: %v\n", m.AllowedPaths)
			}
			fmt.Fprintf(w, "  observed: %s — %s\n", m.Kind, m.Reason)
		}
		for _, m := range g.IdentityCandidates {
			fmt.Fprintf(w, "  identity candidate: %s / %s — %s\n", m.SessionID, m.IntentID, m.Reason)
		}
		for _, m := range g.Suggestions {
			fmt.Fprintf(w, "  suggestion: %s / %s (observed %s) — %s\n", m.SessionID, m.IntentID, m.ObservedState, m.Reason)
		}
		for _, gap := range g.Gaps {
			fmt.Fprintf(w, "  gap: %s\n", gap)
		}
		fmt.Fprintf(w, "  next: %s\n", g.NextStep)
	}
	_, err := fmt.Fprintf(w, "%d groups shown; all groups retained in JSON. No lifecycle state changed. No Git/provider calls.\n", shown)
	return err
}

func priorityRank(p string) int {
	switch p {
	case "EVIDENCE_READY":
		return 0
	case "FOLLOW_UP_CANDIDATE":
		return 1
	case "CONCRETE_CANDIDATE":
		return 2
	default:
		return 3
	}
}

func uniqueMatches(in []IntentMatch) []IntentMatch {
	out := []IntentMatch{}
	seen := map[string]bool{}
	for _, m := range in {
		data, _ := json.Marshal(m)
		key := string(data)
		if !seen[key] {
			out = append(out, m)
			seen[key] = true
		}
	}
	return out
}

func threadIdentity(s *model.Session) string {
	if s.ProviderSessionID != "" {
		return s.ProviderSessionID
	}
	return s.ID
}
func descriptionIdentity(s *model.Session, i *model.Intent) string {
	return model.ID("description-", s.HostID, s.Harness, threadIdentity(s), strings.Join(strings.Fields(i.Description), " "))
}
func unknownGroupKey(s *model.Session, i *model.Intent) string {
	// Context and kind changes remain separate even when descriptions match.
	payload, _ := json.Marshal(struct {
		Description, Kind, Origin, TargetRepoID string
		Explicit                                bool
		Files, Symbols, Commands                []string
		Completion                              []model.CriterionGroup
		Tracker                                 *model.TrackerSubject
		Artifacts                               []model.RequestedArtifact `json:"Artifacts,omitempty"`
	}{strings.Join(strings.Fields(i.Description), " "), i.Kind, i.Origin, i.TargetRepoID, i.Explicit, i.Files, i.Symbols, i.Commands, i.Completion, i.Tracker, i.Artifacts})
	if s.HostID == "" || strings.TrimSpace(i.Description) == "" {
		return model.ID("unknown-", s.ID, i.ID, string(payload))
	}
	return model.ID("unknown-", s.HostID, s.Harness, threadIdentity(s), scope(s), string(payload))
}

const abortedControl = "<turn_aborted>\nThe user interrupted the previous turn on purpose. Any running unified exec processes may still be running in the background. If any tools/commands were aborted, they may have partially executed.\n</turn_aborted>"

func controlOnly(i *model.Intent) bool { return strings.TrimSpace(i.Description) == abortedControl }
func typedControl(i *model.Intent) bool {
	return controlOnly(i) && i.Origin == "harness_control" && i.SourceEvent != nil && exactEvent(*i.SourceEvent, i.Description, "harness_control")
}

func residualObservation(i *model.Intent) bool {
	return i.Origin == "residual_dirty_observation" && boundEvent(i) && i.SourceEvent.Kind == "assistant_message"
}
func observationOnly(i *model.Intent) bool { return controlOnly(i) || residualObservation(i) }
