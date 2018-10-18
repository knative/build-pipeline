# Contributing to Pipeline CRD

Welcome to the Pipeline CRD project! Thanks for considering contributing to our project and we hope you'll enjoy it :D

**All contributors must comply with [the code of conduct](./code-of-condut.md).**

To get started developing, see our [DEVELOPMENT.md](./DEVELOPMMENT.md).

In this file you'll find info on:

* [Principles](#principles)
* [The pull request process](#pull-request-process) and [Prow commands](#prow-commands)
* [Standards](#standards) around [commit messages](#commit-messages) and [code](#coding-standards)
* [The roadmap and contributions wanted](#roadmap-and-contributions-wanted)
* [Contacting other contributors](#contact)

_See also [the contribution guidelines for Knative](https://github.com/knative/docs/blob/master/community/CONTRIBUTING.md)._

## Principles

When possbile, try to practice:

* **Documentation driven development** - Before implementing anything, write docs to explain
  how it will work
* **Test driven development** - Before implementing anything, write tests to cover it

Minimize the number of integration tests written and maximize the unit tests! Unit test
coverage should increase or stay the same with every PR.

This means that most PRs should include both:

* Tests
* Documentation explaining features being added, including updates to [DEVELOPMENT.md](./DEVELOPMENT.md) if required

## Pull Request Process

This repo uses [Prow](https://github.com/kubernetes/test-infra/tree/master/prow)
and related tools like [Tide](https://github.com/kubernetes/test-infra/tree/master/prow/tide)
and [Gubernator](https://github.com/kubernetes/test-infra/tree/master/gubernator)
(see [Knative Prow](https://github.com/knative/test-infra/blob/master/ci/prow/prow_setup.md)).
This means that automation will be applied to your pull requests.

_See also [Knative docs on reviewing](https://github.com/knative/docs/blob/master/community/REVIEWING.md)._

### Prow configuration

Prow is configured in [the knative `config.yaml` in `knative/test-infra`](https://github.com/knative/test-infra/blob/master/ci/prow/config.yaml)
via the sections for `knative/build-pipeline`.

### Prow commands

Prow has a [number of commands](https://prow.knative.dev/command-help) you can use to interact with it.
More on the Prow proces in general
[is available in the k8s docs](https://github.com/kubernetes/community/blob/master/contributors/guide/owners.md#the-code-review-process).

#### Getting sign off

Before a PR can be merged, it must have both `/lgtm` AND `/approve`:

* `/lgtm` can be added by anyone in [the knative org](https://github.com/orgs/knative/people)
* `/approve` can be added only by [OWNERS](https://github.com/knative/build-pipeline/blob/master/OWNERS)

[OWNERS](https://github.com/knative/build-pipeline/blob/master/OWNERS) automatically get `/approve`
but still will need an `/lgtm` to merge.

The merge will happen automatically once the PR has both `/lgtm` and `/approve`,
and all tests pass. If you don't want this to happen you should [`/hold`](#preventing-the-merge)
the PR.

Any changes will cause the `/lgtm` label to be removed and it will need to be re-applied.

#### Preventing the merge

If you don't a PR to be merged, e.g. so that the author can make changes, you can put it on hold with `/hold`.
Remove the hold with `/hold cancel`.

#### Tests

If you are a member of [the knative org](https://github.com/orgs/knative/people), tests (required to merge)
will be automatically kicked off for your PR. If not,
[someone with `/lgtm` or `/approve` permission](#getting-sign-off)
will need to add `/ok-to-test` to your PR to allow the tests to run.

When tests (run by Prow) fail, it will add a comment to the PR with the commands to re-run any failing tests

#### Cats and dogs

You can add dog and cat pictures with `/woof` and `/meow`.

## Standards

This section describes the standards we will try to maintain in this repo.

### Commit Messages

All commit messages should follow [these best practices](https://chris.beams.io/posts/git-commit/),
specifically:

* Start with a subject line
* Contain a body that explains _why_ you're making the change you're making
* Reference an issue number one exists, closing it if applicable (with text such as
  ["Fixes #245" or "Closes #111"](https://help.github.com/articles/closing-issues-using-keywords/))

Aim for [2 paragraphs in the body](https://www.youtube.com/watch?v=PJjmw9TRB7s).
Not sure what to put? Include:

* What is the problem being solved?
* Why is this the best approach?
* What other approaches did you consider?
* What side effects will this approach have?
* What future work remains to be done?

### Coding standards

The code in this repo should follow best practices, specifically:

* [Go code review comments](https://github.com/golang/go/wiki/CodeReviewComments)

## Roadmap and contributions wanted

As of Sept 2018, our roadmap for the next few months is to:

1. Soldify the API, including:

   * [Designing conditional execution](https://github.com/knative/build-pipeline/issues/27)
   * [Designing sources](https://github.com/knative/build-pipeline/issues/13)
   * [Ensuring the the same inputs/outputs are used throughout the pipeline](https://github.com/knative/build-pipeline/issues/11)
   * [Design references between objects](https://github.com/knative/build-pipeline/issues/38)
   * [Design templating](https://github.com/knative/build-pipeline/issues/36)
   * [Design results](https://github.com/knative/build-pipeline/issues/37)

2. Complete a user study (see issues labeled with [user-study](https://github.com/knative/build-pipeline/issues?q=is%3Aissue+is%3Aopen+label%3Auser-study))

3. [Create an initial POC command line tool for interacting with Pipelines](https://github.com/knative/build-pipeline/issues/35)

### Contributions wanted

* To find issues that we particularly would like contributors to tackle, look for
  [issues with the "help wanted" label](https://github.com/knative/build-pipeline/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22).
* Issues that are good for new folks will additionally be marked with
  ["good first issue"](https://github.com/knative/build-pipeline/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22).

### Project board

Our project board is available at: https://github.com/knative/build-pipeline/projects/1
The columns on the board are:

* Ice box: Work we would like to do, but don't plan to tackle in the next couple weeks
* Blocked: Issues that are blocked by other tasks

## Contact

This work is being done by
[the Build CRD working group](https://github.com/knative/docs/blob/master/community/WORKING-GROUPS.md#build).
If you are interested please join our meetings and or in slack at
[`#build-pipeline`](https://knative.slack.com/messages/build-pipeline)!

All docs shared with this group are made visible to members of
[knative-dev@](https://groups.google.com/forum/#!forum/knative-dev), please join if you are interested!
