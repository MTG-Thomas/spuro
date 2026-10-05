package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const SchemaVersion = 1
const Version = "0.2.0-dev"

type Reachability string

const (
	DurableRef     Reachability = "DURABLE_REF"
	RemoteTracking Reachability = "REMOTE_TRACKING"
	WorktreeHead   Reachability = "WORKTREE_HEAD"
	StashRoot      Reachability = "STASH"
	ReflogOnly     Reachability = "REFLOG_ONLY"
	Unreachable    Reachability = "UNREACHABLE"
	Unknown        Reachability = "UNKNOWN"
)

type Equivalence string

const (
	ExactlyPreserved    Equivalence = "EXACTLY_PRESERVED"
	TreeEquivalent      Equivalence = "TREE_EQUIVALENT"
	PatchEquivalent     Equivalence = "PATCH_EQUIVALENT"
	PartiallySuperseded Equivalence = "PARTIALLY_SUPERSEDED"
	Unique              Equivalence = "UNIQUE"
	EquivalenceUnknown  Equivalence = "UNKNOWN"
)

type Severity string

const (
	PreserveFirst Severity = "PRESERVE_FIRST"
	Review        Severity = "REVIEW"
	Housekeeping  Severity = "HOUSEKEEPING"
	Info          Severity = "INFO"
)

type Confidence string

const (
	High   Confidence = "high"
	Medium Confidence = "medium"
	Low    Confidence = "low"
)

type Result struct {
	SchemaVersion int          `json:"schema_version"`
	Scan          Scan         `json:"scan"`
	Repositories  []Repository `json:"repositories"`
	Findings      []Finding    `json:"findings"`
	Plugins       []PluginRun  `json:"plugins"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
}
type Scan struct {
	Version     string             `json:"version"`
	GitVersion  string             `json:"git_version"`
	Roots       []string           `json:"roots"`
	Started     time.Time          `json:"started"`
	Finished    time.Time          `json:"finished"`
	Complete    bool               `json:"complete"`
	MaxWorkers  int                `json:"max_workers"`
	Timings     map[string]float64 `json:"timings_seconds"`
	Excludes    []string           `json:"excludes"`
	Limitations []string           `json:"limitations"`
}
type Repository struct {
	ID                string        `json:"id"`
	CommonGitDir      string        `json:"common_git_dir"`
	Path              string        `json:"observation_path"`
	Bare              bool          `json:"bare"`
	ObjectFormat      string        `json:"object_format"`
	Complete          bool          `json:"complete"`
	ChangedDuringScan bool          `json:"changed_during_scan"`
	Shallow           bool          `json:"shallow"`
	Partial           bool          `json:"partial"`
	Primary           string        `json:"primary_ref,omitempty"`
	PrimaryOID        string        `json:"primary_oid,omitempty"`
	Checkouts         []Checkout    `json:"checkouts"`
	Remotes           []Remote      `json:"remotes"`
	Refs              []Ref         `json:"refs"`
	Branches          []Branch      `json:"branches"`
	Stashes           []Stash       `json:"stashes"`
	Reflog            []ReflogEntry `json:"reflog"`
	Commits           []Commit      `json:"commits"`
	Lineages          []Lineage     `json:"lineages"`
	Evidence          []Evidence    `json:"evidence"`
	Diagnostics       []Diagnostic  `json:"diagnostics"`
}
type Checkout struct {
	Path         string        `json:"path"`
	RepoID       string        `json:"repo_id"`
	GitDir       string        `json:"git_dir"`
	CommonGitDir string        `json:"common_git_dir"`
	Branch       string        `json:"branch"`
	HeadOID      string        `json:"head_oid"`
	Detached     bool          `json:"detached"`
	Main         bool          `json:"main"`
	Registered   bool          `json:"registered"`
	Exists       bool          `json:"exists"`
	Valid        bool          `json:"valid"`
	Locked       bool          `json:"locked"`
	Prunable     bool          `json:"git_prunable"`
	Reason       string        `json:"registration_reason,omitempty"`
	StatusKnown  bool          `json:"status_known"`
	Modified     []PathChange  `json:"modified"`
	Staged       []PathChange  `json:"staged"`
	Untracked    []string      `json:"untracked"`
	Conflicted   []string      `json:"conflicted"`
	Files        []FileVersion `json:"file_versions"`
	Index        []IndexEntry  `json:"index_entries"`
}
type PathChange struct {
	Path         string `json:"path"`
	OriginalPath string `json:"original_path,omitempty"`
	Status       string `json:"status"`
	HeadOID      string `json:"head_oid,omitempty"`
	IndexOID     string `json:"index_oid,omitempty"`
	Submodule    string `json:"submodule,omitempty"`
}
type FileVersion struct {
	Path          string    `json:"path"`
	OID           string    `json:"oid,omitempty"`
	Size          int64     `json:"size"`
	Modified      time.Time `json:"modified"`
	Kind          string    `json:"kind"`
	Stable        bool      `json:"stable"`
	Known         bool      `json:"known"`
	DurableCopies []Copy    `json:"durable_copies"`
	RepresentedBy []Copy    `json:"represented_by"`
}
type IndexEntry struct {
	Path          string `json:"path"`
	OID           string `json:"oid"`
	Stage         int    `json:"stage"`
	Mode          string `json:"mode"`
	DurableCopies []Copy `json:"durable_copies"`
}
type Remote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	HEAD string `json:"head,omitempty"`
}
type Ref struct {
	Name       string       `json:"name"`
	OID        string       `json:"oid"`
	CommitOID  string       `json:"commit_oid,omitempty"`
	ObjectType string       `json:"object_type"`
	Upstream   string       `json:"upstream,omitempty"`
	Symbolic   string       `json:"symbolic,omitempty"`
	Timestamp  time.Time    `json:"timestamp"`
	Class      Reachability `json:"class"`
}
type Branch struct {
	Name            string `json:"name"`
	OID             string `json:"oid"`
	Upstream        string `json:"upstream"`
	UpstreamOID     string `json:"upstream_oid,omitempty"`
	Ahead           int    `json:"ahead"`
	Behind          int    `json:"behind"`
	CountsKnown     bool   `json:"counts_known"`
	UpstreamGone    bool   `json:"upstream_gone"`
	RemoteReachable bool   `json:"remote_reachable"`
	LineageID       string `json:"lineage_id,omitempty"`
}
type Stash struct {
	Ref       string    `json:"ref"`
	OID       string    `json:"oid"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}
type ReflogEntry struct {
	OID       string    `json:"oid"`
	Ref       string    `json:"ref"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}
type Commit struct {
	OID          string         `json:"oid"`
	TreeOID      string         `json:"tree_oid"`
	Parents      []string       `json:"parents"`
	Subject      string         `json:"subject"`
	Timestamp    time.Time      `json:"timestamp"`
	Reachability Reachability   `json:"reachability"`
	Classes      []Reachability `json:"reachability_classes"`
	PatchID      string         `json:"patch_id,omitempty"`
}
type Copy struct {
	RepoID string `json:"repo_id"`
	Ref    string `json:"ref,omitempty"`
	OID    string `json:"oid,omitempty"`
	Kind   string `json:"kind"`
}
type Lineage struct {
	ID                 string        `json:"id"`
	Kind               string        `json:"kind"`
	Ref                string        `json:"ref,omitempty"`
	Checkout           string        `json:"checkout,omitempty"`
	TipOID             string        `json:"tip_oid"`
	Reachability       Reachability  `json:"reachability"`
	Primary            string        `json:"primary_ref,omitempty"`
	MergeBase          string        `json:"merge_base,omitempty"`
	Commits            []string      `json:"commits"`
	SHAUnique          int           `json:"sha_unique"`
	PatchUnique        int           `json:"patch_unique"`
	PatchUnknown       int           `json:"patch_unknown"`
	Equivalence        Equivalence   `json:"equivalence"`
	EquivalentCopies   []Copy        `json:"equivalent_copies"`
	RepresentedCommits []string      `json:"represented_commits"`
	DiffStat           string        `json:"diff_stat,omitempty"`
	StashLike          bool          `json:"stash_like"`
	StashConfidence    Confidence    `json:"stash_confidence,omitempty"`
	StashFiles         []FileVersion `json:"stash_files"`
	UntrackedParent    string        `json:"untracked_parent,omitempty"`
	Complete           bool          `json:"complete"`
}
type Evidence struct {
	ID              string         `json:"id"`
	Provider        string         `json:"provider"`
	ProviderVersion string         `json:"provider_version,omitempty"`
	Operation       string         `json:"operation"`
	Fact            string         `json:"fact"`
	Subject         string         `json:"subject,omitempty"`
	Data            map[string]any `json:"data,omitempty"`
}
type Finding struct {
	ID              string     `json:"id"`
	Severity        Severity   `json:"severity"`
	Kind            string     `json:"kind"`
	RepoID          string     `json:"repo_id,omitempty"`
	Checkout        string     `json:"checkout,omitempty"`
	Ref             string     `json:"ref,omitempty"`
	OID             string     `json:"oid,omitempty"`
	Paths           []string   `json:"paths"`
	Summary         string     `json:"summary"`
	Evidence        []string   `json:"evidence"`
	Confidence      Confidence `json:"confidence"`
	SuggestedAction string     `json:"suggested_action"`
}
type Diagnostic struct {
	Provider   string `json:"provider"`
	Operation  string `json:"operation"`
	RepoID     string `json:"repo_id,omitempty"`
	Path       string `json:"path,omitempty"`
	Message    string `json:"message"`
	Incomplete bool   `json:"incomplete"`
}
type PluginRun struct {
	Name            string  `json:"name"`
	Version         string  `json:"version,omitempty"`
	Status          string  `json:"status"`
	Observations    int     `json:"observations"`
	DurationSeconds float64 `json:"duration_seconds"`
	Message         string  `json:"message,omitempty"`
}

func ID(prefix string, parts ...string) string {
	// Length-free separators cannot alias because JSON/paths/refnames cannot contain NUL.
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + hex.EncodeToString(sum[:12])
}
func RepoID(path string) string { return ID("repo-", path) }
