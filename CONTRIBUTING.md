# Contributing

Keep changes small, explainable, and read-only. Native Git facts outrank plugins.

Run `make check build` before publishing. Tests construct disposable repositories;
never run mutation fixtures against a developer source estate. Keep raw estate
reports, private paths, commit subjects, and file contents outside this repository.

Use signed commits. New preservation claims need fixture tests, explicit evidence,
and documented limits. Prefer the standard library; justify new dependencies.
Cross-platform behavior is tested on Linux, macOS, and Windows. POSIX-only plugin
fixture tests are skipped on Windows; do not interpret that as full plugin coverage.

Tag releases only after CI and vulnerability checks pass. See [release instructions](docs/releasing.md).
