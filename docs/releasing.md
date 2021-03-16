# Releasing

We use [GoReleaser](https://goreleaser.com/) to generate release artifacts, which is initiated when a tag pushed to the repository.

## Tagging
The tag we use in the repository follows [semver](https://github.com/semver/semver/blob/master/semver.md) with the leading **v**. We want the tag to be signed by the person creating it.

``` shell
git checkout <branch>
git fetch
git reset --hard origin/<branch>
git tag -s -a vMajor.Minor.patch[-(alpha,beta,rc).#]
git push origin <tag>
```

## New minor release
When we are ready to release a new minor version of `wksctl`, we will need a branch to enable changes to the minor release and enable work to continue on the next release.
1. Create a branch named release-Major.Minor, e.g., `release-0.8`. Notice the branch doesn't have the leading v or include the patch number.
1. Work with Weaveworks corp to make this branch protected and require PR reviews.

When creating changes for the release, we prefer they are merged into the master branch and then cherry-picked to the release branch. If the master branch has changed to where this isn't possible or practical, the changeset can be merged into the release branch. If you encounter this situation, please open a ticket to ensure the changes are re-applied to the master branch, and we don't have a regression in future releases.

## Patch release
Follow the tagging process identified. The intention is for the minor release to have patch builds, and we won't use release branches for patch releases.  i.e., the branch is moving forward, and we will create versioned patch releases as necessary.


