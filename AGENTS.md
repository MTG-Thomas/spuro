# Agent instructions

Spuro is a read-only preservation-risk scanner. Inspect state before edits and
preserve unrelated work. Never introduce repository mutation into scanning or
plugins. Invoke Git with argv, use machine-readable formats, and keep native
observations separate from plugin assertions and synthesized findings.

Use signed branches and ready-for-review PRs for changes; no direct main pushes.
Sign commits. Before publishing run `make check build`; verify exact signatures.
Changes to safety, reachability, equivalence, discovery, or parsing require fixture
coverage. Never publish real estate reports or machine-specific private evidence.
Keep README concise, current maturity in docs/maturity.md, and gaps in docs/roadmap.md.

Licensing is AGPL-3.0-only. Do not infer licensing from reference repositories or
change it without an explicit owner instruction.

## Secret gate before publication

Before publishing the current PR branch, commit the complete candidate and run
`python3 scripts/secret_gate.py` (native Windows: `python`). Require exit 0;
missing tooling/authentication, scan errors and skipped scans block publication.
Then run the existing repository gates. Repeat after changing outgoing commits.
Push exactly the scanned HEAD. This PR scan covers the current branch against its
fetched intended base; other refs, multi-ref pushes and rewritten history require
`--pre-push` with Git's complete ref-update stream. Stop if any pushed ref is
uncovered. See [secret scanning](SECRET-SCANNING.md) for base selection, new branches,
pre-push input and prerequisites. Preserve GitGuardian App, CI and Gitleaks checks.

Local tooling: `make pre-push`.
