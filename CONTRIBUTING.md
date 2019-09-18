# Contributing to the project

Our doc on [developing `wksctl`](https://wksctl.readthedocs.io/en/latest/development.html) will be helpful when getting started contributing as well.

## Typical workflow

1. Choose an issue from the backlog
1. Fork the repo
1. Update source, test, and documentation files
1. Make sure all the unit tests pass and all the files successfully lint.  I've taken to running `make clean all unit-tests install`
1. Commit and push your new branch
1. Open a pull request against the master branch in the main repo
1. Address comments and questions in pull request
1. Merge to main repo
1. Delete your branch

## Unit Testing

For all new functionality, provide unit tests where practical and possible.  Of course, when they are needed is subjective and might be different for everyone.  When trying to decide, keep in mind that a more comprehensive unit test suite makes it easier for engineers to work in areas of the code they are unfamilar with.

## Integration Testing

We have a set of integration tests which run as part of the Circle CI build.  It's OK to push changes to your branch and let CircleCI run the integration tests.  When changing/adding significant functionality, please add integration tests.

## Pull Requests

Create pull requests for your branch early during development and push code frequently.  Pull requests are an efficient mechanism to keep other engineers informed about the work you are doing and gives them an opportunity to weigh in before the formal review is requested.

When opening the PR, put a description of the feature/functionality along with `fixes #<issuenumber>`.  This will move the issue to done automatically when the pull request is closed.  Additionally, if there are descrete pieces of work in your PR, consider adding a [task list](https://guides.github.com/features/mastering-markdown/#GitHub-flavored-markdown) identifying the work.  That takes the form of

> - [ ] add foo
> - [ ] add bar

GitHub will add x of y to the issue when it is displayed in the project board.

### Reviews

We only require a single maintainer to approve your PR prior to merging it.  While you can identify multiple maintainers, it is assumed you **require** everyone listed to review the PR.  If you just want to make sure other engineers take a look, use the GitHub [mentions](https://guides.github.com/features/mastering-markdown/#GitHub-flavored-markdown) `@<username>` within a comment and they will receive a notification about your PR.

### Merging

One of the maintainers will merge your PR after it is approved.

## Releasing a new version

Once the release has been created and the binaries are available, add the new version of wksctl to the checkpoint system at <https://checkpoint-api.weave.works/admin>
