# Releasing

Spuro follows haloCLI's scripted release strategy, adapted to native Go archives:
clean/version-matched preflight → signed tag → CI tests/audits/SBOM/package → verify
published downloads. The scanner and plugins remain read-only; `scripts/release.py`
is a separate developer release operator. Python 3.10+, Go, Git, Make and authenticated
`gh` are needed for local release operation. No local binary upload token is needed.

## Prepare through a PR

Update `internal/model/model.go`, the changelog and `.github/releases/vX.Y.Z.md`
together on a signed branch. Merge the PR after checks/reviews pass, then update a
clean local `main`. Development versions such as `0.3.0-dev` cannot be tagged by the
operator. The release commit must equal remote main, have a valid local signature,
be GitHub Verified, and have successful **main-push** `ci` and `govulncheck` runs for
that exact commit. Pending or stale checks do not qualify.

```sh
python3 scripts/release.py plan 0.3.0       # preflight/plan only, no tag or publication
python3 scripts/release.py publish 0.3.0    # local gates, signed tag, CI watch, verification
python3 scripts/release.py verify-existing 0.3.0
```

`publish` reruns `make check build`, rechecks identity, creates/verifies a signed
annotated tag at the captured commit, pushes it, watches the newly created exact-tag
release workflow, and verifies downloaded release assets. It does not bypass signing,
force refs, install Spuro, or alter another repository/feed. If delivery is uncertain,
inspect the exact tag/run/release before retrying. Never replace tags or published
assets to repair a release; fix forward to a new version.

## CI packaging and rehearsal

The release workflow handles `v*.*.*` tags and can be manually dispatched with an
existing signed tag. An empty dispatch tag runs a **non-publishing preview**. Changes
to release code/workflow/docs on a PR also rehearse all six archive builds and their
verification; PR events cannot run the publication job.

The workflow checks the signed tag's GitHub verification, exact checkout, main
ancestry and exact main-push gates. It runs full local gates, the release fixtures,
and source vulnerability analysis. Pinned release-only tools are installed into the
runner's temporary directory: govulncheck v1.1.4 and cyclonedx-gomod v1.12.0. They are
not Spuro runtime dependencies. The CycloneDX SBOM records aggregate module and Go
standard-library dependencies; it is not a per-platform runtime coverage claim.

Native OS runners build Linux/macOS/Windows archives for amd64 and arm64, containing
Spuro, both adapters, LICENSE, README and `BUILD.json`. Every binary is audited using
its compiled symbols, and embedded VCS/target/CGO settings must match. Executables
matching the runner's native architecture are smoke-tested: Spuro version/help and
adapter manifests. Cross-compiled targets are marked `runtime_smoke:false`; their
runtime behavior remains unverified, rather than assumed from a native companion.

Publication waits for every build and aggregate verification. Release assets are
six archives, six platform build receipts, `spuro-sbom.cdx.json`, and `SHA256SUMS`.
Receipts include exact source, compiler, target, per-binary build info/digests and
producer workflow run/attempt. Verification reads the complete downloaded set,
checks archive/sidecar equality, recomputes binary digests, and independently reads
embedded build info without running downloaded executables. The publisher performs
this before publication and repeats verification against the actual published
downloads. A failure after publication is reported, not erased by deleting assets.

Checksums and build receipts establish consistency, not independent binary signing,
trusted attestations or a durable backup. Older v0.1/v0.2 assets lack the new receipts
and SBOM; strict `verify-existing` refuses them. Existing assets stay untouched.
Native Git is installed separately; analyzer adapters are optional.

## Source and intentional differences

Reviewed haloCLI at commit `2a33938337a4198c6cdcf2e59c371f8f73cdcbc2`:
[release checklist](https://github.com/Midtown-Technology-Group/halocli/blob/2a33938337a4198c6cdcf2e59c371f8f73cdcbc2/RELEASE.md),
[release workflow](https://github.com/Midtown-Technology-Group/halocli/blob/2a33938337a4198c6cdcf2e59c371f8f73cdcbc2/.github/workflows/release.yml),
[release operator](https://github.com/Midtown-Technology-Group/halocli/blob/2a33938337a4198c6cdcf2e59c371f8f73cdcbc2/scripts/release.py).

Adopted: explicit preflight, CI-produced packages, tests/audit/SBOM, actual-binary
smoke checks, dry-run plan and read-only downloaded-asset rehearsal. Spuro retains
signed commits/tags and pins exact-head evidence. Halo's unsigned-tag override,
Windows-specific local paths, MSI/WinGet cross-repo rewriting/deployment and local
install steps are not copied. Adding an MSI or enrolling Spuro in a package feed is
separate installer/distribution work, with its own identities, permissions and tests.
