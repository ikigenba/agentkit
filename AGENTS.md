# AgentKit

## Versioning

Versions are specified **only** as annotated git tags in the form `vMAJOR.MINOR.PATCH`
(e.g. `v0.1.4`). There is no `VERSION` file or version constant in the code.

- The version is the plain tag — `v0.1.4` — never a `git describe` string.
  Output like `v0.1.3-30-g1b9110d` is git synthesizing "30 commits past `v0.1.3`
  at hash `1b9110d`"; it is **not** a version this project uses. No commit count,
  no `g<hash>` suffix.
- To cut a release: commit the work, then `git tag -a vX.Y.Z -m "vX.Y.Z"` on `main`.
- The current/latest version is whatever `git tag --sort=-v:refname | head -1` reports.
