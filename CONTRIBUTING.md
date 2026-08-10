# Contributing

## Workflow

1. Work from one linked GitHub issue.
2. Use `issue/<id>-<slug>` for the branch.
3. Keep at most one open pull request in this repository.
4. Add focused RED tests before behavioral changes when TDD adds value, and
   record the observed failure in normal Git or pull-request evidence.
5. Run `./scripts/ci_local.sh`.
6. Link admissible public evidence for every profile fact.
7. Squash merge only after CI and review gates pass.

Profile, detector, codec, qualification, applicability, or observable behavior
changes trigger the public doc-gate. Modbus framing and transport behavior
belong in `helianthus-modbus`, not here.

## Evidence And Licensing

Do not submit credentials, private network details, customer identifiers,
restricted source material, or private-repository artifacts. By contributing,
you confirm the material can be published under this repository's AGPL-3.0
license or, for independently authored implementation-neutral protocol facts,
the designated CC0-1.0 documentation lane.

See the
[Helianthus licensing model](https://github.com/Project-Helianthus/.github/blob/main/LICENSING.md).
