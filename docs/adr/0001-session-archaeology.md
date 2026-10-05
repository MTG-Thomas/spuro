# ADR 0001: Session archaeology and implementation-tightening amendment

Status: accepted design; implementation staged. Date: 2026-10-04.

## Decision

Extend Spuro with optional, local, read-only session archaeology: correlate agent
intent with native Git/filesystem evidence. Prefer normalized external providers,
beginning with deja-vu; do not build universal transcript parsers. Providers supply
observations, Spuro owns correlation. No automatic resumption, agent invocation,
tasks/issues, commits, cleanup, transcript rewriting, or uploads.

## Accepted amendment

A. Completion is per concrete intent. Each intent has observable criteria and an
independent state. Derive session summaries afterward; changed files and passing
commands only prove their specific observed facts. A session with two satisfied
intents and one unresolved intent is PARTIALLY_COMPLETED, not COMPLETED.

B. Record GitHistory, SessionHistory, WorkingTree, and ProviderHistory coverage
separately as COMPLETE, PARTIAL, UNKNOWN, or UNAVAILABLE, with scope and reasons.
Partial later coverage prevents FAILED_AND_ABANDONED; use FAILED_UNRESOLVED or
UNKNOWN. COMPLETE is scoped evidence completeness, never universal visibility.

C. Primary intent states: COMPLETED, SUPERSEDED, RESUMED_LATER, UNRESOLVED,
FAILED_UNRESOLVED, PROMISED_NOT_OBSERVED, DIRTY_STATE_REMAINS, UNKNOWN.
Session summaries: COMPLETED, PARTIALLY_COMPLETED, UNRESOLVED, MIXED, UNKNOWN.
Explicit cancellation can supersede a requirement; narration is not a requirement.

D. Built-in adapters expose semantic ListSessions/ReadSession operations with exact,
reviewed argv allowlists. No install/bootstrap, config changes, transcript scrub,
Git-note writing, arbitrary forwarded arguments, or unclear-side-effect commands.
Prefer existing-index queries; disable unsupported capabilities with diagnostics.

E. read_only manifests are advisory. Spuro owns safety policy. Third-party protocol
JSON is untrusted; no shell grants. External executables retain user privileges,
so a manifest cannot establish safety or replace independent command review.

F. Coverage and confidence are independent. High confidence in a command failure
can coexist with partial later coverage; never combine them into one score.

G. Pipeline: session → concrete intents → observable criteria → later Git/session/
filesystem evidence → per-intent states → derived session summaries. Deterministic
completion outranks semantic incompletion, but completing one criterion must not
silently complete unrelated intents. Missing/unstable evidence blocks absence claims.

## Privacy and estate boundaries

Store metadata, concise necessary/redacted evidence, and provider references, not
full transcripts. Support text-free reports. No external semantic API dependency.
Host-qualified identities, checkout/index-state distinctions, and preservation-copy
versus verified-backup distinctions apply to sessions as to repository archaeology.
Provider failure must not affect native scanning. Preserve schema-v1 scan consumers;
introduce a versioned session envelope before any schema-v2 migration.

## Implementation order

1. Commit this ADR and amendment before substantive implementation.
2. Define normalized Intent, IntentState, Coverage, and SessionSummary models.
3. Review pinned current upstream CLI paths and define exact safe deja-vu queries.
4. Implement only that allowlisted adapter surface.
5. Add provider fixtures before live integration.
6. Implement deterministic intent correlation with synthetic Git/session fixtures.
7. Add derived session summaries last.

Fixtures must cover partial completion, later completion, cancellation, linked
continuations, dirty/index state, patch-equivalent history, partial coverage,
malformed/unavailable providers, and command-specific side-effect refusal.
