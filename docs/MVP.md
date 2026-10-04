# Spuro MVP implementation contract

Spuro is a read-only preservation-risk scanner, implemented in Go with native Git as the authoritative backend. The supplied Spuro MVP specification governs this implementation.

The pipeline is discovery, observation, normalization, correlation, classification and presentation. Repository identity is the canonical common Git directory. Independent clones retain independent identities, but equivalent content may be correlated across the entire scan.

Milestones:

1. CLI, recursive discovery including nested/bare repositories, registered worktrees, porcelain status, upstreams and schema-v1 JSON.
2. Ref/HEAD/stash reachability, tree equality, cached stable patch IDs and lineage comparisons. Candidate branches exclude their own ref from equivalence claims; self-patches are never equivalents.
3. Reflog ancestry, both read-only fsck modes, candidate tip reduction, stash-structure analysis including third-parent file versions, and explainable risk findings.
4. Executable JSON plugin protocol, verified reporting adapters, safe opt-in custom providers, failure isolation and provenance. Plugins never replace native facts or independently elevate preservation severity.
5. Concise human output, documentation, fixture regression coverage, mutation-invariance checks and a fresh scan of the previously audited estate.

Safety: argv-only subprocesses; a restrictive read-command gate; optional locks, lazy fetch, fsmonitor, external diffs/textconv and automatic maintenance disabled. No scan writes to roots. Output destinations beneath discovered repositories or roots are refused. Tests create and mutate only isolated fixture repositories. Plugin trust declarations are contracts, not a sandbox; unknown unverified commands are not even invoked for their manifest.

Absence claims are scoped to the observed normal-ref content index, with explicit coverage and concurrency diagnostics. A stable patch ID ignores whitespace and is weaker than byte-identical tree/blob preservation. Merge commits, empty patches, incomplete histories, symlink/submodule changes and unavailable content cannot silently become patch-equivalent. No squash or semantic obsolescence claim is made.

Reachability records separate local durable refs, remote-tracking refs, checkout HEADs, stash refs, reflog ancestry and unreachable objects. A local branch can be retained by its own ref yet warrant review because no equivalent exists elsewhere. No-upstream alone is informational.

No cleanup, rescue refs, commits, pushes, remote ancestry queries, persistent cache, LLM integration, daemon or UI are in the scanning MVP. Any development commit in this new project must be signed.
