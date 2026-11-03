# Contributor migration

This is a breaking source-layout change. The repository is now
`kisinga/jbl-bar-toolkit`. Existing release asset names and installed device paths
stay intact; old GitHub links redirect.

| Previous source | Current source |
| --- | --- |
| `webremote/` | `apps/remote/` |
| `webremote/native/`, device scripts | `models/bar91/remote/` |
| `ssh/`, `smart-mode/` | `models/bar91/ssh/`, `models/bar91/smart-mode/` |
| Model tests and validation | `tests/bar91/`, `models/bar91/validation/` |

For an open branch, commit/stash your work, fetch the renamed origin and rebase
onto main **after this PR merges**. Apply conflicts at the new paths; do not
restore the old source tree. For local clones, update the remote if needed:

```sh
git remote set-url origin https://github.com/kisinga/jbl-bar-toolkit.git
```

`tools/build_*.py` remain available. Use `./jbl dev`, `./jbl test` and `./jbl build`
as the main entry points. Go stays one module; native feature files retain their
names. Upstream through `3a52f58`, including PRs #5, #7, #8 and #9, is incorporated.

USB packages now enter through a shared automatic dispatcher. Config remains
`JBLToolkit/config.txt`. Keep an existing installation's original USB/recovery;
cross-version updating is still pending. No firmware eligibility was broadened.
