# Licence notice bundle design

**Approved by the user on 2026-11-11.** Implementation starts only after PR #34 integrates `50509e6` and the confidential-recovery branch through `5767ba2`.

## Scope

Package the existing evidence in `supply-chain/LICENSE-REVIEW.md` for exactly the 81 packaging-concern rows in its addendum: 77 Metadata-only rows plus four Sharp compound rows. This is not a new legal review, legal-clearance claim, or exhaustive native/bundled-component audit.

- **47 direct `gitHead`-pinned records:** exact upstream commit and notice text with SHA-256.
- **21 tag-based candidates:** seven Next.js and 14 Rolldown records. Their exact tags and resolved commits are qualified context, not direct npm-artifact-to-commit proof.
- **13 unresolved Sharp/native records:** nine libvips plus four compound records. Keep missing declared/bundled notices explicitly unresolved.

## Evidence model

Add a tracked JSON manifest covering all 81 exact name/version rows. Each row contains its declaration, evidence class, upstream source, and notice references. Each included notice is a tracked UTF-8 text file with SHA-256. Tag-based rows carry an explicit qualification; unresolved rows carry a limitation and no invented text.

Add a deterministic `supply-chain/THIRD-PARTY-NOTICES.txt` generated from the manifest. It distinguishes direct, tag-based and unresolved records and says that upstream copies do not prove the texts shipped in npm tarballs.

## Validation and CI

Offline validation must fail closed for malformed/duplicate records, packages absent or at another version in `web/pnpm-lock.yaml`, missing notice files, wrong SHA-256, direct/tag rows without notices, tag rows without qualification, unresolved rows claiming resolved evidence, and generated-output drift. Production data must have exactly 47 direct, 21 tag-based and 13 unresolved rows. The 13 documented unresolved rows do not by themselves fail the build; they must remain visible and must not be represented as resolved.

Run the checker from `scripts/supply-chain.sh generate` and `verify`, with CI contract coverage. Verification requires no network or package execution.

## Provenance and documentation

After the implementation is committed on a clean tree, regenerate scanner evidence with the pinned Trivy 0.76.2 and current source revision using `./scripts/supply-chain.sh generate`; then run `./scripts/supply-chain.sh verify`. Do not hand-edit scanner revision metadata. Update `supply-chain/README.md` and the review with bundle scope, validation command and remaining limitations.
