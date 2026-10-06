package model

import "time"

// ContextEvidence contains offline reports of concrete user/tracker observations.
// Its producer's claims are advisory; exact matching never overrides Git facts.
type ContextEvidence struct {
	ScopedReverts    []ScopedRevert   `json:"scoped_reverts,omitempty"`
	StashArtifacts   []StashArtifact  `json:"stash_artifacts,omitempty"`
	SchemaVersion    int              `json:"schema_version"`
	BoundedRequests  []BoundedRequest `json:"bounded_requests"`
	TrackerWitnesses []TrackerWitness `json:"tracker_witnesses"`
}
type ContextProvenance struct {
	Provider   string    `json:"provider"`
	Artifact   string    `json:"artifact"`
	RecordID   string    `json:"record_id"`
	ObservedAt time.Time `json:"observed_at"`
	Evidence   []string  `json:"evidence"`
}
type BoundedRequest struct {
	ID                 string            `json:"id"`
	Target             IntentRef         `json:"target"`
	Actor              string            `json:"actor"`
	At                 time.Time         `json:"at"`
	HostID             string            `json:"host_id"`
	RepoID             string            `json:"repo_id"`
	AllowedPaths       []string          `json:"allowed_paths"`
	OtherWorkForbidden bool              `json:"other_work_forbidden"`
	Provenance         ContextProvenance `json:"provenance"`
}
type TrackerSubject struct {
	ForgeHost  string `json:"forge_host"`
	Repository string `json:"repository"` // Producer-supplied canonical forge repository identity.
	Kind       string `json:"kind"`
	Number     int    `json:"number"`
	Action     string `json:"action"`
}
type TrackerWitness struct {
	ID         string            `json:"id"`
	Target     IntentRef         `json:"target"`
	Subject    TrackerSubject    `json:"subject"`
	Outcome    string            `json:"outcome"`
	EventAt    time.Time         `json:"event_at"`
	CheckedBy  string            `json:"checked_by"`
	Provenance ContextProvenance `json:"provenance"`
}

// IntentEvent binds the retained intent description to a specific source event.
// The producer must independently match the actual payload; hashes prove only
// internal consistency, not source authenticity or semantic completion.
type IntentEvent struct {
	Kind              string            `json:"kind"`
	At                time.Time         `json:"at"`
	Match             string            `json:"match"`
	DescriptionSHA256 string            `json:"description_sha256"`
	Provenance        ContextProvenance `json:"provenance"`
}
type TargetResolution struct {
	HostID     string            `json:"host_id"`
	RepoID     string            `json:"repo_id"`
	Method     string            `json:"method"` // explicit_repository or verified_user_path; never cwd fallback.
	Provenance ContextProvenance `json:"provenance"`
}

// ScopedRevert is a supplied user instruction, not proof it was executed.
type ScopedRevert struct {
	ID       string      `json:"id"`
	Target   IntentRef   `json:"target"`
	HostID   string      `json:"host_id"`
	ThreadID string      `json:"thread_id"`
	Actor    string      `json:"actor"`
	Request  string      `json:"request"`
	Event    IntentEvent `json:"event"`
}

// StashArtifact is a producer-checked blob witness at an exact untracked parent.
// It does not prove the requested version or a durable copy exists.
type StashArtifact struct {
	ID           string            `json:"id"`
	Target       IntentRef         `json:"target"`
	HostID       string            `json:"host_id"`
	RepoID       string            `json:"repo_id"`
	CommonGitDir string            `json:"common_git_dir"`
	StashOID     string            `json:"stash_oid"`
	ParentOID    string            `json:"parent_oid"`
	ParentNumber int               `json:"parent_number"`
	Path         string            `json:"path"`
	BlobOID      string            `json:"blob_oid"`
	ObjectFormat string            `json:"object_format"`
	ObjectType   string            `json:"object_type"`
	CheckedAt    time.Time         `json:"checked_at"`
	Provenance   ContextProvenance `json:"provenance"`
}
type RequestedArtifact struct {
	Path            string `json:"path"`
	ExpectedBlobOID string `json:"expected_blob_oid,omitempty"`
}
