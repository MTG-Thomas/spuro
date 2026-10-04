# Agent instructions

Spuro is a read-only preservation-risk scanner. Inspect state before edits and
preserve unrelated work. Never introduce repository mutation into scanning or
plugins. Invoke Git with argv, use machine-readable formats, and keep native
observations separate from plugin assertions and synthesized findings.

Sign commits. Before publishing run `make check build`; verify exact signatures.
Changes to safety, reachability, equivalence, discovery, or parsing require fixture
coverage. Never publish real estate reports or machine-specific private evidence.
Keep README concise, current maturity in docs/maturity.md, and gaps in docs/roadmap.md.
