# Verified maturity

Version 0.1.1 is an early release, not a completeness guarantee. The Linux native
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

Development main (0.2.0-dev) has unreleased session artifact ingestion and synthetic
correlation fixtures. No live-session corpus validation or provider refresh was
performed. See [session usage and coverage limits](sessions.md).

The unreleased reconciliation report has a synthetic 2,000-candidate regression
fixture: background UNKNOWN records remain in JSON without flooding the default
shortlist. Session phase timings are instrumented; the reported Windows latency
has not been independently profiled or reproduced locally.
