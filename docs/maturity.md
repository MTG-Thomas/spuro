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


Version 0.3.0 incorporates peer-reported Windows offline feedback, using
fully invented schema-sighting, bounded-request and exact PR-target fixtures.
The peer reported 2,165 UNKNOWN sightings across five cached reports and 11
cross-group same-thread description suggestions; these are not 11 self-group
failures. The implementation host has not independently inspected that corpus.
The new binary's private Windows acceptance remains pending. Context contracts
require actual event provenance; producer normalization and source authenticity
remain external responsibilities. These tests do not establish historic intent
completion or comprehensive cross-machine coverage.


The Windows peer subsequently reported batch01 validation complete for `dc9e377`:
verified binary/signature, 12 independent synthetic regressions and successful
Linux/macOS/Windows CI. With explicitly corrected producer harness identity, all
2,165 sightings remained in 1,876 groups; no same-thread exact-description activity
suggestions remained. This is peer-reported private acceptance, not inspection of
private artifacts from Linux or automatic harness-alias inference.

Batch02 development fixtures cover exact control boilerplate versus real requests,
scoped revert chronology/negation/identity, residual observation provenance, and
per-artifact stash third-parent/source-version boundaries. Private Windows batch02
acceptance remains pending; supplied source authenticity is not established here.


The Windows peer reported batch02 acceptance for `a9218f3` (32 independent fixtures)
and batch03 acceptance for `55e1f87` (38 independent fixtures), verified exact build
hashes/signatures and green exact-head CI. Batch04 adds synthetic inherited-origin
fixtures with missing/contradictory provenance negatives; private acceptance remains
pending. Full-event and fragment extraction authenticity remains producer-supplied.

The release workflow now has a non-publishing six-target packaging rehearsal,
source and compiled-binary vulnerability gates, module/stdlib CycloneDX inventory,
embedded build receipts, and downloaded-asset verification. Published v0.2.0
assets are unchanged; the new asset contract begins with v0.3.0. Cross-compiled
targets are explicitly marked when not runtime-smoked.
See [release operations](releasing.md) for the guarantees and boundaries.
