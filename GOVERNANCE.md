# Project governance

Steward is maintained by [@z-chenhao](https://github.com/z-chenhao).
The repository owner makes final scope, compatibility and release decisions.
Contributors propose changes through issues and pull requests; no committee or
unpublished roadmap is implied.

## Decisions and review

- Describe the user or engineering problem, evidence, proposed behavior and tradeoffs.
- Discuss public interfaces, data migrations, authorization and major architectural changes before implementation.
- Keep one coherent change per PR. Small fixes do not require a separate issue.
- Preserve approval, frozen snapshots, durable replay and credential boundaries.
- Reviews should address the work respectfully. Maintainers may moderate abusive
  or off-topic content; report abuse through GitHub's reporting controls.
- Submitted contributions are intended under the project's Apache-2.0 license;
  retain third-party notices and only contribute material you can license.

## Branches and pull requests

`main` is the integration branch. Work on short-lived topic branches or forks.
The required `Ubuntu checks` status and the active main ruleset remain in place;
CI changes must preserve this check name. A green build does not authorize a release.

Delete a topic branch after its PR has merged and the branch has no newer commits
or other open PRs. Squash merges need the PR's recorded head and merge commit as
evidence; ancestry of the topic head alone is insufficient. Close superseded PRs
with a clear replacement recorded in the PR. Do not delete unmerged work merely
because it is old, conflicting or has failing checks.

The owner may explicitly retire unmerged branches. Before deletion, record exact
heads and open PRs, preserve the commits in a Git bundle, and verify that bundle
can be cloned independently and passes `git fsck`. Retain the bundle and ref
manifest outside tracked documentation. Delete only the authorized refs using
exact-head leases; if a head changes or an active PR needs the branch, review it
again. Retirement does not imply the commits were merged.

## Releases and maintenance

Version tags and publication require the owner's explicit authorization. Native
packaging creates checksummed artifacts and unpublished draft releases. Review
extended regressions, race checks, native-platform results, dependency/license
changes and migration/rollback implications before publishing. Current platform
limits are in the [installation guide](docs/features/installation.md).

Dependency update PRs receive normal review. In particular, changing a runtime or
model bridge dependency can affect frozen capabilities and replay. There is no
automatic dependency merge or promise of long-term support for historical builds.
