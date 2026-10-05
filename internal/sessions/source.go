// Package sessions consumes existing artifacts. It never executes a provider.
package sessions

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	gitbackend "github.com/MTG-Thomas/spuro/internal/git"
	"github.com/MTG-Thomas/spuro/internal/model"
)

const MaxArtifactBytes = 256 << 20
const MaxRecordBytes = 8 << 20

type SourceArtifact struct {
	SHA256 string `json:"sha256,omitempty"`

	Path            string     `json:"path"`
	Type            string     `json:"type"`
	Provider        string     `json:"provider"`
	ProducerVersion string     `json:"producer_version,omitempty"`
	FormatVersion   string     `json:"format_version"`
	HostID          string     `json:"host_id"`
	Modified        time.Time  `json:"modified"`
	Size            int64      `json:"size"`
	MayBeStale      bool       `json:"may_be_stale"`
	Earliest        *time.Time `json:"earliest,omitempty"`
	Latest          *time.Time `json:"latest,omitempty"`
	RetentionGaps   []string   `json:"retention_gaps"`
}

type DejaVuSource interface {
	Discover() ([]SourceArtifact, error)
	ReadSessions(context.Context, SourceArtifact) ([]model.Session, error)
}

// FileSource permits only explicitly named files; it has no executable field.
type FileSource struct {
	Paths                           []string
	Format, ProducerVersion, HostID string
}

func (f FileSource) Discover() ([]SourceArtifact, error) {
	if f.Format != "normalized" && f.Format != "deja-sync" {
		return nil, fmt.Errorf("unsupported artifact format %q", f.Format)
	}
	if f.Format == "deja-sync" && f.ProducerVersion != "0.21.5" && f.ProducerVersion != "0.21.6" {
		return nil, fmt.Errorf("deja sync export requires reviewed producer version 0.21.5 or 0.21.6; no embedded schema version is available")
	}
	out := []SourceArtifact{}
	seen := map[string]bool{}
	for _, p := range f.Paths {
		a, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		a, err = filepath.EvalSymlinks(a)
		if err != nil {
			return nil, err
		}
		if seen[a] {
			continue
		}
		seen[a] = true
		stat, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !stat.Mode().IsRegular() || stat.Size() > MaxArtifactBytes {
			return nil, fmt.Errorf("artifact must be a bounded regular file: %s", a)
		}
		provider, version := "normalized-json", "1"
		if f.Format == "deja-sync" {
			provider = "deja-vu"
			version = "unversioned-sync-jsonl/reviewed-" + f.ProducerVersion
		}
		out = append(out, SourceArtifact{Path: a, Type: f.Format, Provider: provider, ProducerVersion: f.ProducerVersion, FormatVersion: version, HostID: f.HostID, Modified: stat.ModTime().UTC(), Size: stat.Size(), MayBeStale: true, RetentionGaps: []string{"artifact may be incremental; skipped harnesses, retention, and later sessions are unknown"}})
	}
	return out, nil
}

type NormalizedArtifact struct {
	SchemaVersion int             `json:"schema_version"`
	Sessions      []model.Session `json:"sessions"`
}

func strict(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing or malformed JSON")
	}
	return nil
}
func (f FileSource) ReadSessions(ctx context.Context, src SourceArtifact) ([]model.Session, error) {
	if src.Type != f.Format {
		return nil, fmt.Errorf("artifact type does not match source policy")
	}
	if _, err := (FileSource{Format: f.Format, ProducerVersion: f.ProducerVersion}).Discover(); err != nil {
		return nil, err
	}
	file, err := os.Open(src.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() != src.Size || !before.ModTime().Equal(src.Modified) {
		return nil, fmt.Errorf("artifact changed since discovery")
	}
	hasher := sha256.New()
	reader := io.TeeReader(file, hasher)
	var ss []model.Session
	if src.Type == "normalized" {
		data, err := io.ReadAll(io.LimitReader(reader, MaxArtifactBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > MaxArtifactBytes {
			return nil, fmt.Errorf("artifact size limit exceeded")
		}
		var a NormalizedArtifact
		if err := strict(data, &a); err != nil {
			return nil, err
		}
		if a.SchemaVersion != 1 {
			return nil, fmt.Errorf("unsupported normalized session schema %d", a.SchemaVersion)
		}
		ss = a.Sessions
	} else {
		ss, err = readSync(ctx, reader, src)
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	pathNow, err := os.Stat(src.Path)
	if err != nil {
		return nil, err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(before, pathNow) || !pathNow.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("artifact changed while reading")
	}
	seen := map[string]bool{}
	for i := range ss {
		s := &ss[i]
		if s.ID == "" || strings.ContainsRune(s.ID, 0) || seen[s.ID] {
			return nil, fmt.Errorf("missing or duplicate session ID")
		}
		seen[s.ID] = true
		if err := validateSession(*s); err != nil {
			return nil, err
		}
		s.Provider = src.Provider
		// Provider-supplied provenance is not an observed file-read receipt.
		s.Sources = nil
		earliest, latest := s.FirstObservedAt, s.LastObservedAt
		if earliest == nil {
			earliest = s.StartedAt
		}
		if latest == nil {
			latest = s.EndedAt
		}
		source := model.SessionSource{SHA256: fmt.Sprintf("%x", hasher.Sum(nil)), Provider: src.Provider, FormatVersion: src.FormatVersion, ProducerVersion: src.ProducerVersion, Path: src.Path, Type: src.Type, Modified: src.Modified, MayBeStale: true, Earliest: earliest, Latest: latest, RetentionGaps: src.RetentionGaps}
		if s.HostID == "" {
			s.HostID = src.HostID
		}
		s.Coverage = normalizeCoverage(s.Coverage)
		s.Coverage.ProviderHistory = model.CoverageObservation{Level: model.CoveragePartial, Scope: src.Path, Until: &src.Modified, Reasons: append([]string{fmt.Sprintf("existing artifact mtime %s; not refreshed; generation time and completeness unproven", src.Modified.Format(time.RFC3339))}, src.RetentionGaps...)}
		s.Sources = append(s.Sources, source)
	}
	return ss, nil
}

func normalizeCoverage(c model.Coverage) model.Coverage {
	for _, p := range []*model.CoverageObservation{&c.GitHistory, &c.SessionHistory, &c.WorkingTree, &c.ProviderHistory} {
		if p.Level == "" {
			p.Level = model.CoverageUnknown
		}
		if p.Level == model.CoverageComplete && p.Scope == "" {
			p.Level = model.CoverageUnknown
			p.Reasons = append(p.Reasons, "unscoped completeness rejected")
		}
	}
	return c
}
func validateSession(s model.Session) error {
	if err := ValidateContext(s.Context); err != nil {
		return err
	}
	if s.StartedAt != nil && s.EndedAt != nil && s.EndedAt.Before(*s.StartedAt) {
		return fmt.Errorf("session end precedes start")
	}
	if len(s.Intents) > 1000 || len(s.Commands) > 10000 {
		return fmt.Errorf("session observation limit exceeded")
	}
	for _, level := range []model.CoverageLevel{s.Coverage.GitHistory.Level, s.Coverage.WorkingTree.Level, s.Coverage.SessionHistory.Level, s.Coverage.ProviderHistory.Level} {
		if level != "" && level != model.CoverageComplete && level != model.CoveragePartial && level != model.CoverageUnknown && level != model.CoverageUnavailable {
			return fmt.Errorf("invalid coverage level")
		}
	}
	commands := map[string]bool{}
	for _, c := range s.Commands {
		if c.ID == "" || strings.ContainsRune(c.ID, 0) || commands[c.ID] {
			return fmt.Errorf("invalid command identity")
		}
		commands[c.ID] = true
	}
	ids := map[string]bool{}
	for _, i := range s.Intents {
		if i.ID == "" || strings.ContainsRune(i.ID, 0) || ids[i.ID] || i.SessionID != s.ID {
			return fmt.Errorf("invalid intent identity in session %q", s.ID)
		}
		ids[i.ID] = true
		criteria := map[string]bool{}
		for _, group := range i.Completion {
			if group.Mode != "ALL" && group.Mode != "ANY" || len(group.Criteria) == 0 {
				return fmt.Errorf("invalid completion group")
			}
			for _, c := range group.Criteria {
				if c.ID == "" || criteria[c.ID] {
					return fmt.Errorf("duplicate/empty criterion ID")
				}
				criteria[c.ID] = true
			}
		}
	}
	return nil
}

type syncRecord struct {
	Harness   string    `json:"harness"`
	SessionID string    `json:"session_id"`
	Project   string    `json:"project"`
	Role      string    `json:"role"`
	Text      string    `json:"text"`
	Time      time.Time `json:"time"`
	Origin    string    `json:"origin,omitempty"`
}

func readSync(ctx context.Context, r io.Reader, src SourceArtifact) ([]model.Session, error) {
	scan := bufio.NewScanner(io.LimitReader(r, MaxArtifactBytes+1))
	scan.Buffer(make([]byte, 64<<10), MaxRecordBytes)
	byID := map[string]*model.Session{}
	tail := map[string]syncRecord{}
	lines := 0
	total := 0
	for scan.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		total += len(scan.Bytes()) + 1
		if total > MaxArtifactBytes {
			return nil, fmt.Errorf("artifact size limit exceeded")
		}
		lines++
		if lines > 100000 {
			return nil, fmt.Errorf("record limit exceeded")
		}
		var rec syncRecord
		if err := strict(scan.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("sync record %d: %w", lines, err)
		}
		if rec.Harness == "" || rec.SessionID == "" || rec.Project == "" || rec.Role == "" || strings.ContainsRune(rec.Harness+rec.SessionID+rec.Project+rec.Origin, 0) {
			return nil, fmt.Errorf("invalid sync record %d", lines)
		}
		id := model.ID("session-", "deja-vu", src.HostID, rec.Origin, rec.Harness, rec.SessionID, rec.Project)
		s := byID[id]
		if s == nil {
			host := src.HostID
			if rec.Origin != "" && rec.Origin != host {
				host = rec.Origin
			}
			s = &model.Session{ID: id, Provider: "deja-vu", ProviderSessionID: rec.SessionID, Harness: rec.Harness, HostID: host, WorkingDir: rec.Project, Coverage: model.UnknownCoverage(), SourceOrigin: rec.Origin}
			byID[id] = s
		}
		if !rec.Time.IsZero() {
			t := rec.Time
			if s.LastObservedAt == nil || t.After(*s.LastObservedAt) {
				s.LastObservedAt = &t
			}
			if s.FirstObservedAt == nil || t.Before(*s.FirstObservedAt) {
				s.FirstObservedAt = &t
			}
		}
		if rec.Role == "assistant" {
			prev, ok := tail[id]
			if !ok || rec.Time.After(prev.Time) || rec.Time.Equal(prev.Time) {
				tail[id] = rec
			}
		}
		// Export timestamps do not establish session termination or executed commands.
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	out := []model.Session{}
	for id, s := range byID {
		if rec, ok := tail[id]; ok {
			extractTail(s, rec.Text)
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

var actionPattern = regexp.MustCompile(`(?i)^(?:todo:|next:|remaining:|still need to:|follow up:|blocked by:)\s*(.+)$`)
var secretPattern = regexp.MustCompile(`(?i)(?:bearer\s+\S+|(?:api[_-]?key|token|password|secret)\s*[:=]\s*\S+|sk-[a-z0-9_-]{8,}|gh[pousr]_[a-z0-9_]{8,})`)

func Redact(s string) string {
	return secretPattern.ReplaceAllString(gitbackend.Redact(s), "[REDACTED]")
}
func extractTail(s *model.Session, text string) {
	for _, line := range strings.Split(text, "\n") {
		match := actionPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 2 {
			continue
		}
		description := Redact(match[1])
		if len(description) > 512 {
			description = description[:512]
		}
		id := model.ID("intent-", s.ID, description)
		duplicate := false
		for _, i := range s.Intents {
			if i.ID == id {
				duplicate = true
			}
		}
		if duplicate {
			continue
		}
		s.Intents = append(s.Intents, model.Intent{ID: id, SessionID: s.ID, Description: description, Kind: "FOLLOW_UP", Explicit: true, Confidence: model.Low})
		if len(s.Intents) >= 16 {
			break
		}
	}
}
