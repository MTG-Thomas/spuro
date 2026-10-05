# Verified maturity

Version 0.2.0 is an early release, not a completeness guarantee. The Linux native
scanner passed unit, race, vet, and private real-estate acceptance checks: 60 logical
repositories, 139 checkout paths, 108 linked worktrees, roughly five minutes.
The acceptance findings included untracked proposal files, conflicted integration
state, and four dropped-stash tips. Private corpus artifacts are not distributed.

CI runs fixture tests and builds on Linux, macOS, and Windows, with race detection
on Linux and a separate vulnerability gate. Tagged releases contain all three
executables for amd64/arm64 on those systems plus SHA-256 checksums.
Platform build support does not prove exhaustive real-estate validation everywhere.

Native Git remains authoritative. Unknown coverage and plugin failures are reported.
Read [validation](validation.md), [schema](schema-v1.md), and [limits](roadmap.md).

Version 0.2.0 adds session artifact ingestion and synthetic
correlation fixtures. No live-session corpus validation or provider refresh was
performed. See [session usage and coverage limits](sessions.md).

The reconciliation report has a synthetic 2,000-candidate regression
fixture: background UNKNOWN records remain in JSON without flooding the default
shortlist. Session phase timings are instrumented; the reported Windows latency
has not been independently profiled or reproduced locally.

Offline UNKNOWN reconciliation is covered by synthetic duplicate sightings,
explicit cancellation, cached commit witnesses, cross-host and ambiguous-scope
negative cases, incomplete chronology, text-free output, and a CLI test with no
Git/provider executable on PATH. The laptop-private candidate corpus was not
accessible from the implementation host; no claim of validating its 2,165 UNKNOWN
sightings is made. Existing snapshots can now be reconciled without a new audit.


Development 0.3.0-dev incorporates peer-reported Windows offline feedback, using
fully invented schema-sighting, bounded-request and exact PR-target fixtures.
The peer reported 2,165 UNKNOWN sightings across five cached reports and 11
cross-group same-thread description suggestions; these are not 11 self-group
failures. The implementation host has not independently inspected that corpus.
The new binary's private Windows acceptance remains pending. Context contracts
require actual event provenance; producer normalization and source authenticity
remain external responsibilities. These tests do not establish historic intent
completion or comprehensive cross-machine coverage.
