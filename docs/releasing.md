# Releasing

Run `make check build`, inspect the exact signed commit, and wait for GitHub CI and
govulncheck to pass. Update the version, changelog, and `.github/releases/vX.Y.Z.md`
together. Create a signed annotated Git tag (`git tag -s vX.Y.Z`) and verify it
(`git verify-tag vX.Y.Z`) before pushing the tag.

The tag workflow builds Linux/macOS/Windows amd64 and arm64 archives containing
Spuro and both optional adapters, LICENSE, and README. It publishes a GitHub
release only after every build succeeds, with SHA256SUMS for downloaded archives.
Checksums verify integrity, not independent binary signing or provenance attestation.
Native Git must be installed separately; external analyzers are optional.
