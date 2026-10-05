package sessions

import (
	"encoding/json"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/model"
)

// Merge combines existing batch sightings without inventing session termination.
// Conflicting normalized intent definitions are rejected rather than resolved by
// whichever artifact happened to be read last.
func Merge(existing, incoming model.Session) (model.Session, error) {
	if existing.ID != incoming.ID || existing.Provider != incoming.Provider || existing.HostID != incoming.HostID || existing.WorkingDir != incoming.WorkingDir {
		return existing, fmt.Errorf("inconsistent session identity across artifacts")
	}
	oldStart := existing.StartedAt
	if existing.Provider == "deja-vu" && incoming.LastObservedAt != nil && existing.LastObservedAt != nil && incoming.LastObservedAt.Equal(*existing.LastObservedAt) {
		a, _ := json.Marshal(existing.Intents)
		b, _ := json.Marshal(incoming.Intents)
		if string(a) != string(b) {
			return existing, fmt.Errorf("ambiguous equal-time sync tails; cannot infer ordering")
		}
	}
	if existing.Provider == "deja-vu" {
		if incoming.LastObservedAt != nil && (existing.LastObservedAt == nil || incoming.LastObservedAt.After(*existing.LastObservedAt)) {
			existing.Intents = incoming.Intents
			existing.LastObservedAt = incoming.LastObservedAt
		}
	} else {
		intents := map[string]model.Intent{}
		for _, i := range existing.Intents {
			intents[i.ID] = i
		}
		for _, i := range incoming.Intents {
			if old, ok := intents[i.ID]; ok {
				a, _ := json.Marshal(old)
				b, _ := json.Marshal(i)
				if string(a) != string(b) {
					return existing, fmt.Errorf("conflicting intent definitions for %s", i.ID)
				}
			} else {
				existing.Intents = append(existing.Intents, i)
				intents[i.ID] = i
			}
		}
		commands := map[string]model.SessionCommand{}
		for _, c := range existing.Commands {
			commands[c.ID] = c
		}
		for _, c := range incoming.Commands {
			if old, ok := commands[c.ID]; ok {
				a, _ := json.Marshal(old)
				b, _ := json.Marshal(c)
				if string(a) != string(b) {
					return existing, fmt.Errorf("conflicting command observations")
				}
			} else {
				existing.Commands = append(existing.Commands, c)
				commands[c.ID] = c
			}
		}
		for _, id := range incoming.Continues {
			found := false
			for _, old := range existing.Continues {
				if old == id {
					found = true
				}
			}
			if !found {
				existing.Continues = append(existing.Continues, id)
			}
		}
		existing.Decisions = append(existing.Decisions, incoming.Decisions...)
		if incoming.EndedAt != nil && (existing.EndedAt == nil || incoming.EndedAt.After(*existing.EndedAt)) {
			existing.EndedAt = incoming.EndedAt
		}
	}
	if incoming.StartedAt != nil && (oldStart == nil || incoming.StartedAt.Before(*oldStart)) {
		existing.StartedAt = incoming.StartedAt
	}
	if incoming.FirstObservedAt != nil && (existing.FirstObservedAt == nil || incoming.FirstObservedAt.Before(*existing.FirstObservedAt)) {
		existing.FirstObservedAt = incoming.FirstObservedAt
	}
	existing.Sources = append(existing.Sources, incoming.Sources...)
	existing.Evidence = append(existing.Evidence, incoming.Evidence...)
	existing.Coverage.ProviderHistory.Level = model.CoveragePartial
	existing.Coverage.ProviderHistory.Reasons = append(existing.Coverage.ProviderHistory.Reasons, "multiple artifact sightings merged; full history still unproven")
	return existing, nil
}
