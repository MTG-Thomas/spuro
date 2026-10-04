# JSON schema v1 contract

Top-level fields: `schema_version` (1), `scan`, `repositories`, `findings`,
`plugins`, `diagnostics`. Empty modeled collections are arrays, not null.
Fields may be added without changing v1; changing field types or enum semantics
requires a schema-version change. Consumers should ignore unknown fields.

The normalized Go structs in `internal/model/model.go` define field names and
types. `scan` records version, Git version, canonical roots, UTC timestamps,
coverage, worker limit, timings, exclusions and limitations. Repository identity
is a SHA-256-derived ID of the canonical common Git directory. It is not a global
identity derived from remotes or repository contents.

Each repository includes checkout/index/file versions; all current refs and
branch/upstream facts; stash/reflog entries; commit OIDs, parents, tree IDs,
subjects, dates, reachability classes and optional patch IDs; and synthesized
lineages with SHA-unique/patch-unique/unknown counts and equivalence copies.
Copy entries are representative ref witnesses, not exhaustive membership lists.
Stash file versions may additionally carry `represented_by` witnesses for tracked
snapshot/aggregate-patch equivalence even when exact blob bytes differ.
Evidence IDs referenced by findings resolve within the repository evidence list.
Filesystem-only failures have no repository ID and refer to global diagnostics.

Reachability priority: `DURABLE_REF`, `REMOTE_TRACKING`, `WORKTREE_HEAD`, `STASH`,
`REFLOG_ONLY`, `UNREACHABLE`; unknown namespaces yield `UNKNOWN` when applicable.
All independently observed reachability classes remain in `reachability_classes`.
Equivalence: `EXACTLY_PRESERVED`, `TREE_EQUIVALENT`, `PATCH_EQUIVALENT`,
`PARTIALLY_SUPERSEDED`, `UNIQUE`, `UNKNOWN`. `UNIQUE` is bounded by the implemented
exact/tree/patch checks and scan scope; it does not assert semantic novelty.

Finding severities: `PRESERVE_FIRST`, `REVIEW`, `HOUSEKEEPING`, `INFO`.
Confidence: `high`, `medium`, `low`, with no artificial percentages.
Finding IDs include repository identity, kind and stable subject attributes.
Scan timestamps and durations naturally differ between scans.

No raw source-file content or patch body is included. Paths, commit subjects,
plugin metadata and tool diagnostics can still contain sensitive information;
keep saved scan reports private. Remote URL userinfo/query credentials are redacted.
