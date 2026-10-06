package sessions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MTG-Thomas/spuro/internal/model"
)

const observedRetryControl = "<turn_aborted>\nThe user interrupted the previous turn on purpose. Any running unified exec processes may still be running in the background. If any tools/commands were aborted, they may have partially executed; verify current state before retrying.\n</turn_aborted>"

func TestRetryControlVariantIsNotIndependentActivity(t *testing.T) {
	r := unknownFixture()
	r.Sessions = r.Sessions[:2]
	for n := range r.Sessions {
		r.Sessions[n].ProviderSessionID = r.Sessions[n].ID
		r.Sessions[n].Intents[0].Description = observedRetryControl
	}
	later := r.Sessions[0].LastObservedAt.Add(time.Hour)
	r.Sessions[1].LastObservedAt = &later
	before, _ := json.Marshal(r)
	q := FindUnknowns(context.Background(), &r)
	if q.Sightings != 2 || len(q.Groups) != 2 {
		t.Fatal("sightings dropped")
	}
	for _, g := range q.Groups {
		if len(g.Suggestions) != 0 || g.ObservationKind != "control_boilerplate_candidate" {
			t.Fatal("retry control supplies activity", g)
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("snapshot/lifecycle state changed")
	}
	i := &r.Sessions[0].Intents[0]
	i.Origin = "harness_control"
	e := eventFixture(i.Description, "harness_control", *r.Sessions[0].LastObservedAt)
	i.SourceEvent = &e
	if !typedControl(i) {
		t.Fatal("exact variant provenance rejected")
	}
	i.SourceEvent.DescriptionSHA256 = strings.Repeat("0", 64)
	if typedControl(i) {
		t.Fatal("invalid provenance promoted")
	}
}
func TestRetryControlVariantRemainsExactAndBounded(t *testing.T) {
	for _, text := range []string{"Explain this example: " + observedRetryControl, observedRetryControl + "\nPlease implement retry tests", strings.Replace(observedRetryControl, "verify current state", "verify repository status", 1)} {
		i := model.Intent{Description: text, Origin: "harness_control"}
		e := eventFixture(text, "harness_control", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		i.SourceEvent = &e
		if controlOnly(&i) || typedControl(&i) {
			t.Fatal("unlisted/substantive text suppressed")
		}
	}
	i := model.Intent{Description: " \n" + observedRetryControl + "\n "}
	if !controlOnly(&i) {
		t.Fatal("outer trim unsupported")
	}
}
