package model

import "time"

// Coverage and Confidence are independent; COMPLETE is always bounded by Scope.
type CoverageLevel string

const (
	CoverageComplete    CoverageLevel = "COMPLETE"
	CoveragePartial     CoverageLevel = "PARTIAL"
	CoverageUnknown     CoverageLevel = "UNKNOWN"
	CoverageUnavailable CoverageLevel = "UNAVAILABLE"
)

type CoverageObservation struct {
	Level    CoverageLevel `json:"level"`
	Scope    string        `json:"scope"`
	Since    *time.Time    `json:"since,omitempty"`
	Until    *time.Time    `json:"until,omitempty"`
	Reasons  []string      `json:"reasons"`
	Evidence []string      `json:"evidence"`
}

type Coverage struct {
	GitHistory      CoverageObservation `json:"git_history"`
	SessionHistory  CoverageObservation `json:"session_history"`
	WorkingTree     CoverageObservation `json:"working_tree"`
	ProviderHistory CoverageObservation `json:"provider_history"`
}

// UnknownCoverage prevents zero values being mistaken for complete observation.
func UnknownCoverage() Coverage {
	unknown := func() CoverageObservation {
		return CoverageObservation{Level: CoverageUnknown, Reasons: []string{}, Evidence: []string{}}
	}
	return Coverage{GitHistory: unknown(), SessionHistory: unknown(), WorkingTree: unknown(), ProviderHistory: unknown()}
}

type IntentState string

const (
	IntentCompleted           IntentState = "COMPLETED"
	IntentSuperseded          IntentState = "SUPERSEDED"
	IntentResumedLater        IntentState = "RESUMED_LATER"
	IntentUnresolved          IntentState = "UNRESOLVED"
	IntentFailedUnresolved    IntentState = "FAILED_UNRESOLVED"
	IntentPromisedNotObserved IntentState = "PROMISED_NOT_OBSERVED"
	IntentDirtyStateRemains   IntentState = "DIRTY_STATE_REMAINS"
	IntentUnknown             IntentState = "UNKNOWN"
)

type SessionSummary string

const (
	SessionCompleted          SessionSummary = "COMPLETED"
	SessionPartiallyCompleted SessionSummary = "PARTIALLY_COMPLETED"
	SessionUnresolved         SessionSummary = "UNRESOLVED"
	SessionMixed              SessionSummary = "MIXED"
	SessionUnknown            SessionSummary = "UNKNOWN"
)

// Criteria are narrow observable predicates, never implicit whole-session goals.
// Any/All relationships are explicit groups so one file change cannot silently
// satisfy several independent requirements.
type CompletionCriterion struct {
	Literal string `json:"literal,omitempty"`

	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	Description          string   `json:"description"`
	Files                []string `json:"files"`
	Symbols              []string `json:"symbols"`
	Commands             []string `json:"commands"`
	ExpectedOID          string   `json:"expected_oid,omitempty"`
	ExpectedObjectFormat string   `json:"expected_object_format,omitempty"`
	Evidence             []string `json:"evidence"`
}

type CriterionGroup struct {
	Mode     string                `json:"mode"` // ALL or ANY, evaluated only by supported predicates.
	Criteria []CompletionCriterion `json:"criteria"`
}

type IntentRef struct {
	SessionID string `json:"session_id"`
	IntentID  string `json:"intent_id"`
}

type Intent struct {
	Artifacts        []RequestedArtifact `json:"requested_artifacts,omitempty"`
	SourceEvent      *IntentEvent        `json:"source_event,omitempty"`
	TargetResolution *TargetResolution   `json:"target_resolution,omitempty"`
	Origin           string              `json:"origin,omitempty"`
	RecordedAt       *time.Time          `json:"recorded_at,omitempty"`
	TargetRepoID     string              `json:"target_repo_id,omitempty"`
	Tracker          *TrackerSubject     `json:"tracker_target,omitempty"`
	Continues        []IntentRef         `json:"continues"`

	ID          string           `json:"id"`
	SessionID   string           `json:"session_id"`
	Description string           `json:"description"`
	Kind        string           `json:"kind"`
	Explicit    bool             `json:"explicit"`
	Files       []string         `json:"files"`
	Symbols     []string         `json:"symbols"`
	Commands    []string         `json:"commands"`
	Completion  []CriterionGroup `json:"completion"`
	Evidence    []string         `json:"evidence"`
	Confidence  Confidence       `json:"confidence"`
}

// An assessment never replaces its original intent or provider observation.
type IntentAssessment struct {
	IntentID           string      `json:"intent_id"`
	State              IntentState `json:"state"`
	Coverage           Coverage    `json:"coverage"`
	Confidence         Confidence  `json:"confidence"`
	SatisfiedCriteria  []string    `json:"satisfied_criteria"`
	UnresolvedCriteria []string    `json:"unresolved_criteria"`
	Evidence           []string    `json:"evidence"`
	RelatedFindings    []string    `json:"related_findings"`
	Reasons            []string    `json:"reasons"`
}

type SessionAssociation struct {
	HostID       string   `json:"host_id"`
	RepoID       string   `json:"repo_id"`
	Checkout     string   `json:"checkout"`
	CommonGitDir string   `json:"common_git_dir"`
	Branch       string   `json:"branch,omitempty"`
	Evidence     []string `json:"evidence"`
}

type SessionCommand struct {
	IntentRefs []IntentRef `json:"intent_refs"`

	IntentIDs []string `json:"intent_ids"`

	ID          string     `json:"id"`
	Command     string     `json:"command,omitempty"`
	WorkingDir  string     `json:"working_dir"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"` // nil means unobserved, never success.
	Interrupted bool       `json:"interrupted"`
	Evidence    []string   `json:"evidence"`
}

type SessionSource struct {
	SHA256 string `json:"sha256"`

	Provider        string     `json:"provider"`
	FormatVersion   string     `json:"format_version"`
	ProducerVersion string     `json:"producer_version,omitempty"`
	Path            string     `json:"path"`
	Type            string     `json:"type"`
	Modified        time.Time  `json:"modified"`
	MayBeStale      bool       `json:"may_be_stale"`
	Earliest        *time.Time `json:"earliest,omitempty"`
	Latest          *time.Time `json:"latest,omitempty"`
	RetentionGaps   []string   `json:"retention_gaps"`
}
type IntentDecision struct {
	SessionID string   `json:"session_id"`
	IntentID  string   `json:"intent_id"`
	Actor     string   `json:"actor"`
	Decision  string   `json:"decision"`
	Evidence  []string `json:"evidence"`
}
type Session struct {
	Context         *ContextEvidence `json:"context_evidence,omitempty"`
	FirstObservedAt *time.Time       `json:"first_observed_at,omitempty"`

	LastObservedAt *time.Time `json:"last_observed_at,omitempty"`
	SourceOrigin   string     `json:"source_origin,omitempty"`

	Sources   []SessionSource  `json:"sources"`
	Decisions []IntentDecision `json:"decisions"`

	ID                string               `json:"id"`
	Provider          string               `json:"provider"`
	ProviderSessionID string               `json:"provider_session_id"`
	Harness           string               `json:"harness"`
	HostID            string               `json:"host_id"`
	StartedAt         *time.Time           `json:"started_at,omitempty"`
	EndedAt           *time.Time           `json:"ended_at,omitempty"`
	WorkingDir        string               `json:"working_dir"`
	Associations      []SessionAssociation `json:"associations"`
	Continues         []string             `json:"continues"`
	Intents           []Intent             `json:"intents"`
	Commands          []SessionCommand     `json:"commands"`
	Coverage          Coverage             `json:"coverage"`
	Evidence          []string             `json:"evidence"`
}

// DerivedSessionAssessment is populated after per-intent correlation, never by
// a provider. Resume links do not assert successful completion.
type DerivedSessionAssessment struct {
	SessionID string             `json:"session_id"`
	Summary   SessionSummary     `json:"summary"`
	Intents   []IntentAssessment `json:"intents"`
	Coverage  Coverage           `json:"coverage"`
	Evidence  []string           `json:"evidence"`
}
