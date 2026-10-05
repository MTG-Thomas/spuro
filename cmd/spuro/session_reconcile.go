package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MTG-Thomas/spuro/internal/model"
	"github.com/MTG-Thomas/spuro/internal/report"
	"github.com/MTG-Thomas/spuro/internal/sessions"
)

// This command is intentionally artifact-only: no native scan, Git runner, or provider.
func runSessionReconcile(ctx context.Context, args []string, out io.Writer) error {
	input, output := "", ""
	asJSON, noText := false, false
	for n := 0; n < len(args); n++ {
		name, value, has := strings.Cut(args[n], "=")
		switch name {
		case "--help", "-h":
			_, err := fmt.Fprintln(out, "Usage: spuro sessions reconcile --input EXISTING_SESSION_JSON [--json] [--no-transcript-text] [--output FILE]\nOffline only: no Git/provider commands or repository scan.")
			return err
		case "--input", "--output":
			if !has {
				n++
				if n >= len(args) {
					return fmt.Errorf("missing value for %s", name)
				}
				value = args[n]
			}
			if name == "--input" {
				input = value
			} else {
				output = value
			}
		case "--json", "--no-transcript-text":
			if has {
				return fmt.Errorf("%s takes no value", name)
			}
			if name == "--json" {
				asJSON = true
			} else {
				noText = true
			}
		default:
			return fmt.Errorf("unknown reconcile option %q", name)
		}
	}
	if input == "" {
		return fmt.Errorf("reconcile requires --input; no scan is performed")
	}
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return err
	}
	const limit = 256 * 1024 * 1024
	if !before.Mode().IsRegular() || before.Size() > limit {
		return fmt.Errorf("reconcile input must be a regular file at most 256 MiB")
	}
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if len(body) > limit {
		return fmt.Errorf("reconcile input exceeds 256 MiB")
	}
	after, err := f.Stat()
	if err != nil {
		return err
	}
	current, err := os.Stat(input)
	if err != nil {
		return err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(before, current) {
		return fmt.Errorf("reconcile input changed during reading")
	}
	var snapshot sessions.Result
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return fmt.Errorf("unsupported session snapshot: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("session snapshot must contain one JSON value")
	}
	if snapshot.Format != "spuro-sessions" || snapshot.SchemaVersion != 1 {
		return fmt.Errorf("reconcile requires spuro-sessions schema 1; private audit schemas are not inferred")
	}
	seenSessions := map[string]bool{}
	intents := map[string]bool{}
	for _, session := range snapshot.Sessions {
		if err := sessions.ValidateContext(session.Context); err != nil {
			return err
		}
		if session.ID == "" || strings.ContainsRune(session.ID, 0) || seenSessions[session.ID] {
			return fmt.Errorf("ambiguous or missing session identity in snapshot")
		}
		seenSessions[session.ID] = true
		for _, intent := range session.Intents {
			key := session.ID + "\x00" + intent.ID
			if intent.ID == "" || strings.ContainsRune(intent.ID, 0) || intents[key] {
				return fmt.Errorf("ambiguous or missing intent identity in snapshot")
			}
			intents[key] = true
		}
	}
	seenAssessments := map[string]bool{}
	for _, assessment := range snapshot.Assessments {
		for _, intent := range assessment.Intents {
			key := assessment.SessionID + "\x00" + intent.IntentID
			if !intents[key] || seenAssessments[key] {
				return fmt.Errorf("dangling or duplicate intent assessment in snapshot")
			}
			seenAssessments[key] = true
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	queue := sessions.FindUnknowns(ctx, &snapshot)
	snapshot.UnknownQueue = &queue
	sessions.Sanitize(&snapshot, noText)
	sum := sha256.Sum256(body)
	envelope := struct {
		Format        string                 `json:"format"`
		Version       string                 `json:"version"`
		SourceVersion string                 `json:"source_version"`
		SchemaVersion int                    `json:"schema_version"`
		SourcePath    string                 `json:"source_path"`
		SourceSHA256  string                 `json:"source_sha256"`
		Queue         *sessions.UnknownQueue `json:"unknown_reconciliation"`
	}{"spuro-session-reconciliation", model.Version, snapshot.Version, 1, input, hex.EncodeToString(sum[:]), snapshot.UnknownQueue}
	render := func(w io.Writer) error {
		if asJSON {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(envelope)
		}
		return sessions.UnknownText(w, *snapshot.UnknownQueue)
	}
	if output == "" {
		return render(out)
	}
	// Protect both the input and every repository/path represented by the original scan.
	protected := model.Result{Scan: snapshot.NativeObservation}
	if snapshot.Native != nil {
		protected = *snapshot.Native
	}
	protected.Scan.Roots = append(append([]string{}, protected.Scan.Roots...), input)
	if protected.Repositories == nil {
		protected.Repositories = []model.Repository{{Checkouts: snapshot.RelatedCheckouts}}
	}
	for _, s := range snapshot.Sources {
		protected.Scan.Roots = append(protected.Scan.Roots, s.Path)
	}
	if target, err := os.Stat(output); err == nil && os.SameFile(before, target) {
		return fmt.Errorf("refusing output over input artifact hardlink")
	}
	return report.WriteOutside(output, &protected, render)
}
