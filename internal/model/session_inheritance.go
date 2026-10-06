package model

// Inheritance evidence is supplied offline provenance, not transcript parsing.
// Fragment hashes are independent of the full-event payload hash.
type InheritanceEvidence struct {
	SchemaVersion int                `json:"schema_version"`
	Forks         []SessionFork      `json:"forks"`
	Events        []OriginEvent      `json:"events"`
	Occurrences   []OriginOccurrence `json:"occurrences"`
	Fragments     []OriginFragment   `json:"fragments"`
}
type OriginIdentity struct {
	HostID   string `json:"host_id"`
	Harness  string `json:"harness"`
	Provider string `json:"provider"`
	ThreadID string `json:"thread_id"`
}
type OriginReceipt struct {
	ArtifactSHA256 string            `json:"artifact_sha256"`
	Provenance     ContextProvenance `json:"provenance"`
}
type SessionFork struct {
	ID string `json:"id"`
	OriginIdentity
	ParentThreadID string `json:"parent_thread_id"`
	OriginReceipt
}
type OriginEvent struct {
	ID string `json:"id"`
	OriginIdentity
	TurnID        string `json:"turn_id"`
	Kind          string `json:"kind"`
	PayloadSHA256 string `json:"payload_sha256"`
	PayloadBytes  int    `json:"payload_bytes"`
	OriginReceipt
}
type OriginOccurrence struct {
	ID string `json:"id"`
	OriginIdentity
	EventID       string `json:"event_id"`
	TurnID        string `json:"turn_id"`
	Kind          string `json:"kind"`
	PayloadSHA256 string `json:"payload_sha256"`
	PayloadBytes  int    `json:"payload_bytes"`
	Inherited     bool   `json:"inherited"`
	OriginReceipt
}
type OriginFragment struct {
	UTF8BoundariesVerified bool      `json:"utf8_boundaries_verified"`
	ID                     string    `json:"id"`
	Target                 IntentRef `json:"target"`
	OccurrenceID           string    `json:"occurrence_id"`
	Match                  string    `json:"match"`
	FragmentSHA256         string    `json:"fragment_sha256"`
	ByteStart              int       `json:"byte_start"`
	ByteEnd                int       `json:"byte_end"`
	OriginReceipt
}
