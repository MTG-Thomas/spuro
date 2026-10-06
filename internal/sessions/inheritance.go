package sessions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MTG-Thomas/spuro/internal/model"
)

func originIdentity(k model.OriginIdentity) string {
	return k.HostID + "\x00" + k.Harness + "\x00" + k.Provider + "\x00" + k.ThreadID
}
func validOriginIdentity(k model.OriginIdentity) bool {
	return k.HostID != "" && k.Harness != "" && k.Provider != "" && k.ThreadID != "" && !strings.ContainsRune(k.HostID+k.Harness+k.Provider+k.ThreadID, 0)
}
func validOriginReceipt(r model.OriginReceipt) bool {
	return validOID(r.ArtifactSHA256, "sha256") && validProvenance(r.Provenance, r.Provenance.ObservedAt)
}
func ValidateInheritance(c *model.InheritanceEvidence) error {
	if c == nil {
		return nil
	}
	if c.SchemaVersion != 1 || len(c.Forks)+len(c.Events)+len(c.Occurrences)+len(c.Fragments) > 10000 {
		return fmt.Errorf("unsupported or oversized inheritance evidence")
	}
	ids := map[string]bool{}
	add := func(id string) bool {
		if id == "" || strings.ContainsRune(id, 0) || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	for _, v := range c.Forks {
		if !add(v.ID) || !validOriginIdentity(v.OriginIdentity) || v.ParentThreadID == "" || strings.ContainsRune(v.ParentThreadID, 0) || !validOriginReceipt(v.OriginReceipt) {
			return fmt.Errorf("invalid fork evidence")
		}
	}
	for _, v := range c.Events {
		if !add(v.ID) || !validOriginIdentity(v.OriginIdentity) || v.TurnID == "" || v.Kind != "task_complete" || !validOID(v.PayloadSHA256, "sha256") || v.PayloadBytes <= 0 || !validOriginReceipt(v.OriginReceipt) {
			return fmt.Errorf("invalid original event evidence")
		}
	}
	for _, v := range c.Occurrences {
		if !add(v.ID) || !validOriginIdentity(v.OriginIdentity) || v.EventID == "" || v.TurnID == "" || v.Kind != "task_complete" || !validOID(v.PayloadSHA256, "sha256") || v.PayloadBytes <= 0 || !validOriginReceipt(v.OriginReceipt) {
			return fmt.Errorf("invalid occurrence evidence")
		}
	}
	for _, v := range c.Fragments {
		if !add(v.ID) || !validTarget(v.Target) || v.OccurrenceID == "" || v.Match != "exact_fragment" || !v.UTF8BoundariesVerified || !validOID(v.FragmentSHA256, "sha256") || v.ByteStart < 0 || v.ByteEnd <= v.ByteStart || !validOriginReceipt(v.OriginReceipt) {
			return fmt.Errorf("invalid fragment evidence")
		}
	}
	return nil
}
func mergeInheritance(a, b *model.InheritanceEvidence) (*model.InheritanceEvidence, error) {
	if err := ValidateInheritance(a); err != nil {
		return nil, err
	}
	if err := ValidateInheritance(b); err != nil {
		return nil, err
	}
	if a == nil {
		return b, nil
	}
	if b == nil {
		return a, nil
	}
	out := &model.InheritanceEvidence{SchemaVersion: 1}
	seen := map[string]string{}
	add := func(id string, v any) (bool, error) {
		data, _ := json.Marshal(v)
		if old, ok := seen[id]; ok {
			if old != string(data) {
				return false, fmt.Errorf("conflicting inheritance record")
			}
			return false, nil
		}
		seen[id] = string(data)
		return true, nil
	}
	for _, c := range []*model.InheritanceEvidence{a, b} {
		for _, v := range c.Forks {
			yes, e := add(v.ID, v)
			if e != nil {
				return nil, e
			}
			if yes {
				out.Forks = append(out.Forks, v)
			}
		}
		for _, v := range c.Events {
			yes, e := add(v.ID, v)
			if e != nil {
				return nil, e
			}
			if yes {
				out.Events = append(out.Events, v)
			}
		}
		for _, v := range c.Occurrences {
			yes, e := add(v.ID, v)
			if e != nil {
				return nil, e
			}
			if yes {
				out.Occurrences = append(out.Occurrences, v)
			}
		}
		for _, v := range c.Fragments {
			yes, e := add(v.ID, v)
			if e != nil {
				return nil, e
			}
			if yes {
				out.Fragments = append(out.Fragments, v)
			}
		}
	}
	return out, ValidateInheritance(out)
}

type checkedOrigin struct {
	key      string
	evidence []string
}
type originIndex struct {
	bindings map[string]checkedOrigin
	gaps     map[string]string
}

func intentRefKey(r model.IntentRef) string { return r.SessionID + "\x00" + r.IntentID }
func indexOrigins(ss []model.Session) originIndex {
	out := originIndex{bindings: map[string]checkedOrigin{}, gaps: map[string]string{}}
	all := &model.InheritanceEvidence{SchemaVersion: 1}
	for _, s := range ss {
		var err error
		all, err = mergeInheritance(all, s.Inheritance)
		if err != nil {
			for _, s := range ss {
				for _, i := range s.Intents {
					out.gaps[s.ID+"\x00"+i.ID] = "conflicting/invalid inherited-origin evidence; no suppression"
				}
			}
			return out
		}
	}
	sessions := map[string]*model.Session{}
	threads := map[string]bool{}
	for n := range ss {
		s := &ss[n]
		sessions[s.ID] = s
		threads[originIdentity(model.OriginIdentity{HostID: s.HostID, Harness: s.Harness, Provider: s.Provider, ThreadID: threadIdentity(s)})] = true
	}
	events := map[string]model.OriginEvent{}
	occ := map[string]model.OriginOccurrence{}
	forks := map[string]string{}
	conflict := map[string]bool{}
	forkEvidence := map[string][]string{}
	for _, e := range all.Events {
		events[e.ID] = e
	}
	for _, o := range all.Occurrences {
		occ[o.ID] = o
	}
	for _, f := range all.Forks {
		k := originIdentity(f.OriginIdentity)
		p := f.OriginIdentity
		p.ThreadID = f.ParentThreadID
		parent := originIdentity(p)
		if old, ok := forks[k]; ok && old != parent {
			conflict[k] = true
		}
		forks[k] = parent
		forkEvidence[k] = append(append([]string{f.ID}, f.Provenance.Evidence...), f.Provenance.RecordID)
	}
	ancestry := func(child, parent string) bool {
		seen := map[string]bool{}
		reached := false
		for {
			if seen[child] || conflict[child] || !threads[child] {
				return false
			}
			seen[child] = true
			if child == parent {
				reached = true
			}
			next, ok := forks[child]
			if !ok {
				return reached
			}
			child = next
		}
	}
	blocked := map[string]bool{}
	for _, f := range all.Fragments {
		if blocked[intentRefKey(f.Target)] {
			continue
		}
		k := intentRefKey(f.Target)
		out.gaps[k] = "missing or incompatible parent event, ancestry, occurrence or exact fragment binding"
		s := sessions[f.Target.SessionID]
		if s == nil {
			continue
		}
		var i *model.Intent
		for n := range s.Intents {
			if s.Intents[n].ID == f.Target.IntentID {
				i = &s.Intents[n]
				break
			}
		}
		if i == nil || i.Description == "" {
			continue
		}
		o, ok := occ[f.OccurrenceID]
		if !ok {
			continue
		}
		e, ok := events[o.EventID]
		if !ok {
			continue
		}
		identity := model.OriginIdentity{HostID: s.HostID, Harness: s.Harness, Provider: s.Provider, ThreadID: threadIdentity(s)}
		if o.OriginIdentity != identity || o.TurnID != e.TurnID || o.Kind != e.Kind || o.PayloadSHA256 != e.PayloadSHA256 || o.PayloadBytes != e.PayloadBytes {
			continue
		}
		child, parent := originIdentity(o.OriginIdentity), originIdentity(e.OriginIdentity)
		if o.Inherited {
			if child == parent || !ancestry(child, parent) {
				continue
			}
		} else if child != parent || !threads[parent] || !ancestry(parent, parent) {
			continue
		}
		sum := sha256.Sum256([]byte(i.Description))
		if f.FragmentSHA256 != hex.EncodeToString(sum[:]) || f.ByteEnd-f.ByteStart != len([]byte(i.Description)) || f.ByteEnd > e.PayloadBytes {
			continue
		}
		origin := checkedOrigin{key: e.ID + "\x00" + f.FragmentSHA256 + fmt.Sprintf("/%d/%d", f.ByteStart, f.ByteEnd), evidence: append(append(append([]string{}, e.Provenance.Evidence...), o.Provenance.Evidence...), f.Provenance.Evidence...)}
		if previous, ok := out.bindings[k]; ok && previous.key != origin.key {
			delete(out.bindings, k)
			blocked[k] = true
			out.gaps[k] = "ambiguous multiple source origins; no suppression"
			continue
		}
		if out.gaps[k] == "ambiguous multiple source origins; no suppression" {
			continue
		}
		for node := child; ; {
			origin.evidence = append(origin.evidence, forkEvidence[node]...)
			next, ok := forks[node]
			if !ok {
				break
			}
			node = next
		}
		out.bindings[k] = origin
		delete(out.gaps, k)
	}
	return out
}
func (x originIndex) same(a, b intentRecord) (bool, []string) {
	left, ok := x.bindings[a.session.ID+"\x00"+a.intent.ID]
	if !ok {
		return false, nil
	}
	right, ok := x.bindings[b.session.ID+"\x00"+b.intent.ID]
	if !ok || left.key != right.key {
		return false, nil
	}
	return true, append(append([]string{}, left.evidence...), right.evidence...)
}
