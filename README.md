# ![pipe](./pipe.png) Tekton Pipelines

[![Go Report Card](https://goreportcard.com/badge/tektoncd/pipeline)](https://goreportcard.com/report/tektoncd/pipeline)

The Tekton Pipelines project provides k8s-style resources for declaring
CI/CD-style pipelines.

Tekton Pipelines are **Cloud Native**:

- Run on Kubernetes
- Have Kubernetes clusters as a first class type
- Use containers as their building blocks

Tekton Pipelines are **Decoupled**:

- One Pipeline can be used to deploy to any k8s cluster
- The Tasks which make up a Pipeline can easily be run in isolation
- Resources such as git repos can easily be swapped between runs

Tekton Pipelines are **Typed**:

- The concept of typed resources means that for a resource such as an `Image`,
  implementations can easily be swapped out (e.g. building with
  [kaniko](https://github.com/GoogleContainerTools/kaniko) v.s.
  [buildkit](https://github.com/moby/buildkit))

## Want to start using Pipelines

- [Installing Tekton Pipelines](docs/install.md)
- Jump in with [the tutorial!](docs/tutorial.md)
- Take a look at our [roadmap](roadmap.md)

*Note that starting from the 0.24 release of Tekton, you need to have
a cluster with **Kubernetes version 1.18 or later***.

### Read the docs

| Version | Docs | Examples |
| ------- | ---- | -------- |
| [HEAD](DEVELOPMENT.md#install-pipeline) | [Docs @ HEAD](/docs/README.md) | [Examples @ HEAD](/examples) |
| [v0.26.0](https://github.com/tektoncd/pipeline/releases/tag/v0.26.0) | [Docs @ v0.26.0](https://github.com/tektoncd/pipeline/tree/v0.26.0/docs#tekton-pipelines) | [Examples @ v0.26.0](https://github.com/tektoncd/pipeline/tree/v0.26.0/examples#examples) |
| [v0.25.0](https://github.com/tektoncd/pipeline/releases/tag/v0.25.0) | [Docs @ v0.25.0](https://github.com/tektoncd/pipeline/tree/v0.25.0/docs#tekton-pipelines) | [Examples @ v0.25.0](https://github.com/tektoncd/pipeline/tree/v0.25.0/examples#examples) |
| [v0.24.3](https://github.com/tektoncd/pipeline/releases/tag/v0.24.3) | [Docs @ v0.24.3](https://github.com/tektoncd/pipeline/tree/v0.24.3/docs#tekton-pipelines) | [Examples @ v0.24.3](https://github.com/tektoncd/pipeline/tree/v0.24.3/examples#examples) |
| [v0.24.2](https://github.com/tektoncd/pipeline/releases/tag/v0.24.2) | [Docs @ v0.24.2](https://github.com/tektoncd/pipeline/tree/v0.24.2/docs#tekton-pipelines) | [Examples @ v0.24.2](https://github.com/tektoncd/pipeline/tree/v0.24.2/examples#examples) |
| [v0.24.1](https://github.com/tektoncd/pipeline/releases/tag/v0.24.1) | [Docs @ v0.24.1](https://github.com/tektoncd/pipeline/tree/v0.24.1/docs#tekton-pipelines) | [Examples @ v0.24.1](https://github.com/tektoncd/pipeline/tree/v0.24.1/examples#examples) |
| [v0.24.0](https://github.com/tektoncd/pipeline/releases/tag/v0.24.0) | [Docs @ v0.24.0](https://github.com/tektoncd/pipeline/tree/v0.24.0/docs#tekton-pipelines) | [Examples @ v0.24.0](https://github.com/tektoncd/pipeline/tree/v0.24.0/examples#examples) |
| [v0.23.0](https://github.com/tektoncd/pipeline/releases/tag/v0.23.0) | [Docs @ v0.23.0](https://github.com/tektoncd/pipeline/tree/v0.23.0/docs#tekton-pipelines) | [Examples @ v0.23.0](https://github.com/tektoncd/pipeline/tree/v0.23.0/examples#examples) |
| [v0.22.0](https://github.com/tektoncd/pipeline/releases/tag/v0.22.0) | [Docs @ v0.22.0](https://github.com/tektoncd/pipeline/tree/v0.22.0/docs#tekton-pipelines) | [Examples @ v0.22.0](https://github.com/tektoncd/pipeline/tree/v0.22.0/examples#examples) |
| [v0.21.0](https://github.com/tektoncd/pipeline/releases/tag/v0.21.0) | [Docs @ v0.21.0](https://github.com/tektoncd/pipeline/tree/v0.21.0/docs#tekton-pipelines) | [Examples @ v0.21.0](https://github.com/tektoncd/pipeline/tree/v0.21.0/examples#examples) |
| [v0.20.1](https://github.com/tektoncd/pipeline/releases/tag/v0.20.1) | [Docs @ v0.20.1](https://github.com/tektoncd/pipeline/tree/v0.20.1/docs#tekton-pipelines) | [Examples @ v0.20.1](https://github.com/tektoncd/pipeline/tree/v0.20.1/examples#examples) |
| [v0.20.0](https://github.com/tektoncd/pipeline/releases/tag/v0.20.0) | [Docs @ v0.20.0](https://github.com/tektoncd/pipeline/tree/v0.20.0/docs#tekton-pipelines) | [Examples @ v0.20.0](https://github.com/tektoncd/pipeline/tree/v0.20.0/examples#examples) |
| [v0.19.0](https://github.com/tektoncd/pipeline/releases/tag/v0.19.0) | [Docs @ v0.19.0](https://github.com/tektoncd/pipeline/tree/v0.19.0/docs#tekton-pipelines) | [Examples @ v0.19.0](https://github.com/tektoncd/pipeline/tree/v0.19.0/examples#examples) |
| [v0.18.1](https://github.com/tektoncd/pipeline/releases/tag/v0.18.1) | [Docs @ v0.18.1](https://github.com/tektoncd/pipeline/tree/v0.18.1/docs#tekton-pipelines) | [Examples @ v0.18.1](https://github.com/tektoncd/pipeline/tree/v0.18.1/examples#examples) |
| [v0.18.0](https://github.com/tektoncd/pipeline/releases/tag/v0.18.0) | [Docs @ v0.18.0](https://github.com/tektoncd/pipeline/tree/v0.18.0/docs#tekton-pipelines) | [Examples @ v0.18.0](https://github.com/tektoncd/pipeline/tree/v0.18.0/examples#examples) |
| [v0.17.3](https://github.com/tektoncd/pipeline/releases/tag/v0.17.3) | [Docs @ v0.17.3](https://github.com/tektoncd/pipeline/tree/v0.17.3/docs#tekton-pipelines) | [Examples @ v0.17.3](https://github.com/tektoncd/pipeline/tree/v0.17.3/examples#examples) |
| [v0.17.2](https://github.com/tektoncd/pipeline/releases/tag/v0.17.2) | [Docs @ v0.17.2](https://github.com/tektoncd/pipeline/tree/v0.17.2/docs#tekton-pipelines) | [Examples @ v0.17.2](https://github.com/tektoncd/pipeline/tree/v0.17.2/examples#examples) |
| [v0.17.1](https://github.com/tektoncd/pipeline/releases/tag/v0.17.1) | [Docs @ v0.17.1](https://github.com/tektoncd/pipeline/tree/v0.17.1/docs#tekton-pipelines) | [Examples @ v0.17.1](https://github.com/tektoncd/pipeline/tree/v0.17.1/examples#examples) |
| [v0.17.0](https://github.com/tektoncd/pipeline/releases/tag/v0.17.0) | [Docs @ v0.17.0](https://github.com/tektoncd/pipeline/tree/v0.17.0/docs#tekton-pipelines) | [Examples @ v0.17.0](https://github.com/tektoncd/pipeline/tree/v0.17.0/examples#examples) |
| [v0.16.3](https://github.com/tektoncd/pipeline/releases/tag/v0.16.3) | [Docs @ v0.16.3](https://github.com/tektoncd/pipeline/tree/v0.16.3/docs#tekton-pipelines) | [Examples @ v0.16.3](https://github.com/tektoncd/pipeline/tree/v0.16.3/examples#examples) |
| [v0.16.2](https://github.com/tektoncd/pipeline/releases/tag/v0.16.2) | [Docs @ v0.16.2](https://github.com/tektoncd/pipeline/tree/v0.16.2/docs#tekton-pipelines) | [Examples @ v0.16.2](https://github.com/tektoncd/pipeline/tree/v0.16.2/examples#examples) |
| [v0.16.1](https://github.com/tektoncd/pipeline/releases/tag/v0.16.1) | [Docs @ v0.16.1](https://github.com/tektoncd/pipeline/tree/v0.16.1/docs#tekton-pipelines) | [Examples @ v0.16.1](https://github.com/tektoncd/pipeline/tree/v0.16.1/examples#examples) |
| [v0.16.0](https://github.com/tektoncd/pipeline/releases/tag/v0.16.0) | [Docs @ v0.16.0](https://github.com/tektoncd/pipeline/tree/v0.16.0/docs#tekton-pipelines) | [Examples @ v0.16.0](https://github.com/tektoncd/pipeline/tree/v0.16.0/examples#examples) |
| [v0.15.2](https://github.com/tektoncd/pipeline/releases/tag/v0.15.2) | [Docs @ v0.15.2](https://github.com/tektoncd/pipeline/tree/v0.15.2/docs#tekton-pipelines) | [Examples @ v0.15.2](https://github.com/tektoncd/pipeline/tree/v0.15.2/examples#examples) |
| [v0.15.1](https://github.com/tektoncd/pipeline/releases/tag/v0.15.1) | [Docs @ v0.15.1](https://github.com/tektoncd/pipeline/tree/v0.15.1/docs#tekton-pipelines) | [Examples @ v0.15.1](https://github.com/tektoncd/pipeline/tree/v0.15.1/examples#examples) |
| [v0.15.0](https://github.com/tektoncd/pipeline/releases/tag/v0.15.0) | [Docs @ v0.15.0](https://github.com/tektoncd/pipeline/tree/v0.15.0/docs#tekton-pipelines) | [Examples @ v0.15.0](https://github.com/tektoncd/pipeline/tree/v0.15.0/examples#examples) |
| [v0.14.3](https://github.com/tektoncd/pipeline/releases/tag/v0.14.3) | [Docs @ v0.14.3](https://github.com/tektoncd/pipeline/tree/v0.14.3/docs#tekton-pipelines) | [Examples @ v0.14.3](https://github.com/tektoncd/pipeline/tree/v0.14.3/examples#examples) |
| [v0.14.2](https://github.com/tektoncd/pipeline/releases/tag/v0.14.2) | [Docs @ v0.14.2](https://github.com/tektoncd/pipeline/tree/v0.14.2/docs#tekton-pipelines) | [Examples @ v0.14.2](https://github.com/tektoncd/pipeline/tree/v0.14.2/examples#examples) |
| [v0.14.1](https://github.com/tektoncd/pipeline/releases/tag/v0.14.1) | [Docs @ v0.14.1](https://github.com/tektoncd/pipeline/tree/v0.14.1/docs#tekton-pipelines) | [Examples @ v0.14.1](https://github.com/tektoncd/pipeline/tree/v0.14.1/examples#examples) |
| [v0.14.0](https://github.com/tektoncd/pipeline/releases/tag/v0.14.0) | [Docs @ v0.14.0](https://github.com/tektoncd/pipeline/tree/v0.14.0/docs#tekton-pipelines) | [Examples @ v0.14.0](https://github.com/tektoncd/pipeline/tree/v0.14.0/examples#examples) |
| [v0.13.2](https://github.com/tektoncd/pipeline/releases/tag/v0.13.2) | [Docs @ v0.13.2](https://github.com/tektoncd/pipeline/tree/v0.13.2/docs#tekton-pipelines) | [Examples @ v0.13.2](https://github.com/tektoncd/pipeline/tree/v0.13.2/examples#examples) |
| [v0.13.1](https://github.com/tektoncd/pipeline/releases/tag/v0.13.1) | [Docs @ v0.13.1](https://github.com/tektoncd/pipeline/tree/v0.13.1/docs#tekton-pipelines) | [Examples @ v0.13.1](https://github.com/tektoncd/pipeline/tree/v0.13.1/examples#examples) |
| [v0.13.0](https://github.com/tektoncd/pipeline/releases/tag/v0.13.0) | [Docs @ v0.13.0](https://github.com/tektoncd/pipeline/tree/v0.13.0/docs#tekton-pipelines) | [Examples @ v0.13.0](https://github.com/tektoncd/pipeline/tree/v0.13.0/examples#examples) |
| [v0.12.1](https://github.com/tektoncd/pipeline/releases/tag/v0.12.1) | [Docs @ v0.12.1](https://github.com/tektoncd/pipeline/tree/v0.12.1/docs#tekton-pipelines) | [Examples @ v0.12.1](https://github.com/tektoncd/pipeline/tree/v0.12.1/examples#examples) |
| [v0.12.0](https://github.com/tektoncd/pipeline/releases/tag/v0.12.0) | [Docs @ v0.12.0](https://github.com/tektoncd/pipeline/tree/v0.12.0/docs#tekton-pipelines) | [Examples @ v0.12.0](https://github.com/tektoncd/pipeline/tree/v0.12.0/examples#examples) |
| [v0.11.3](https://github.com/tektoncd/pipeline/releases/tag/v0.11.3) | [Docs @ v0.11.3](https://github.com/tektoncd/pipeline/tree/v0.11.3/docs#tekton-pipelines) | [Examples @ v0.11.3](https://github.com/tektoncd/pipeline/tree/v0.11.3/examples#examples) |
| [v0.11.2](https://github.com/tektoncd/pipeline/releases/tag/v0.11.2) | [Docs @ v0.11.2](https://github.com/tektoncd/pipeline/tree/v0.11.2/docs#tekton-pipelines) | [Examples @ v0.11.2](https://github.com/tektoncd/pipeline/tree/v0.11.2/examples#examples) |
| [v0.11.1](https://github.com/tektoncd/pipeline/releases/tag/v0.11.1) | [Docs @ v0.11.1](https://github.com/tektoncd/pipeline/tree/v0.11.1/docs#tekton-pipelines) | [Examples @ v0.11.1](https://github.com/tektoncd/pipeline/tree/v0.11.1/examples#examples) |
| [v0.11.0](https://github.com/tektoncd/pipeline/releases/tag/v0.11.0) | [Docs @ v0.11.0](https://github.com/tektoncd/pipeline/tree/v0.11.0/docs#tekton-pipelines) | [Examples @ v0.11.0](https://github.com/tektoncd/pipeline/tree/v0.11.0/examples#examples) |
| [v0.11.0-rc4](https://github.com/tektoncd/pipeline/releases/tag/v0.11.0-rc4) | [Docs @ v0.11.0-rc4](https://github.com/tektoncd/pipeline/tree/v0.11.0-rc4/docs#tekton-pipelines) | [Examples @ v0.11.0-rc4](https://github.com/tektoncd/pipeline/tree/v0.11.0-rc4/examples#examples) |
| [v0.11.0-rc3](https://github.com/tektoncd/pipeline/releases/tag/v0.11.0-rc3) | [Docs @ v0.11.0-rc3](https://github.com/tektoncd/pipeline/tree/v0.11.0-rc3/docs#tekton-pipelines) | [Examples @ v0.11.0-rc3](https://github.com/tektoncd/pipeline/tree/v0.11.0-rc3/examples#examples) |
| [v0.11.0-rc2](https://github.com/tektoncd/pipeline/releases/tag/v0.11.0-rc2) | [Docs @ v0.11.0-rc2](https://github.com/tektoncd/pipeline/tree/v0.11.0-rc2/docs#tekton-pipelines) | [Examples @ v0.11.0-rc2](https://github.com/tektoncd/pipeline/tree/v0.11.0-rc2/examples#examples) |
| [v0.11.0-rc1](https://github.com/tektoncd/pipeline/releases/tag/v0.11.0-rc1) | [Docs @ v0.11.0-rc1](https://github.com/tektoncd/pipeline/tree/v0.11.0-rc1/docs#tekton-pipelines) | [Examples @ v0.11.0-rc1](https://github.com/tektoncd/pipeline/tree/v0.11.0-rc1/examples#examples) |
| [v0.10.2](https://github.com/tektoncd/pipeline/releases/tag/v0.10.2) | [Docs @ v0.10.2](https://github.com/tektoncd/pipeline/tree/v0.10.2/docs#tekton-pipelines) | [Examples @ v0.10.2](https://github.com/tektoncd/pipeline/tree/v0.10.2/examples#examples) |
| [v0.10.1](https://github.com/tektoncd/pipeline/releases/tag/v0.10.1) | [Docs @ v0.10.1](https://github.com/tektoncd/pipeline/tree/v0.10.1/docs#tekton-pipelines) | [Examples @ v0.10.1](https://github.com/tektoncd/pipeline/tree/v0.10.1/examples#examples) |
| [v0.10.0](https://github.com/tektoncd/pipeline/releases/tag/v0.10.0) | [Docs @ v0.10.0](https://github.com/tektoncd/pipeline/tree/v0.10.0/docs#tekton-pipelines) | [Examples @ v0.10.0](https://github.com/tektoncd/pipeline/tree/v0.10.0/examples#examples) |
| [v0.9.2](https://github.com/tektoncd/pipeline/releases/tag/v0.9.2) | [Docs @ v0.9.2](https://github.com/tektoncd/pipeline/tree/v0.9.2/docs#tekton-pipelines) | [Examples @ v0.9.2](https://github.com/tektoncd/pipeline/tree/v0.9.2/examples#examples) |
| [v0.9.1](https://github.com/tektoncd/pipeline/releases/tag/v0.9.1) | [Docs @ v0.9.1](https://github.com/tektoncd/pipeline/tree/v0.9.1/docs#tekton-pipelines) | [Examples @ v0.9.1](https://github.com/tektoncd/pipeline/tree/v0.9.1/examples#examples) |
| [v0.9.0](https://github.com/tektoncd/pipeline/releases/tag/v0.9.0) | [Docs @ v0.9.0](https://github.com/tektoncd/pipeline/tree/v0.9.0/docs#tekton-pipelines) | [Examples @ v0.9.0](https://github.com/tektoncd/pipeline/tree/v0.9.0/examples#examples) |
| [v0.8.0](https://github.com/tektoncd/pipeline/releases/tag/v0.8.0) | [Docs @ v0.8.0](https://github.com/tektoncd/pipeline/tree/v0.8.0/docs#tekton-pipelines) | [Examples @ v0.8.0](https://github.com/tektoncd/pipeline/tree/v0.8.0/examples#examples) |
| [v0.7.0](https://github.com/tektoncd/pipeline/releases/tag/v0.7.0) | [Docs @ v0.7.0](https://github.com/tektoncd/pipeline/tree/v0.7.0/docs#tekton-pipelines) | [Examples @ v0.7.0](https://github.com/tektoncd/pipeline/tree/v0.7.0/examples#examples) |
| [v0.6.0](https://github.com/tektoncd/pipeline/releases/tag/v0.6.0) | [Docs @ v0.6.0](https://github.com/tektoncd/pipeline/tree/release-v0.6.x/docs#tekton-pipelines) | [Examples @ v0.6.0](https://github.com/tektoncd/pipeline/tree/v0.6.0/examples#examples) |
| [v0.5.2](https://github.com/tektoncd/pipeline/releases/tag/v0.5.2) | [Docs @ v0.5.2](https://github.com/tektoncd/pipeline/tree/v0.5.2/docs#tekton-pipelines) | [Examples @ v0.5.2](https://github.com/tektoncd/pipeline/tree/v0.5.2/examples#examples) |
| [v0.5.1](https://github.com/tektoncd/pipeline/releases/tag/v0.5.1) | [Docs @ v0.5.1](https://github.com/tektoncd/pipeline/tree/v0.5.1/docs#tekton-pipelines) | [Examples @ v0.5.1](https://github.com/tektoncd/pipeline/tree/v0.5.1/examples#examples) |
| [v0.5.0](https://github.com/tektoncd/pipeline/releases/tag/v0.5.0) | [Docs @ v0.5.0](https://github.com/tektoncd/pipeline/tree/v0.5.0/docs#tekton-pipelines) | [Examples @ v0.5.0](https://github.com/tektoncd/pipeline/tree/v0.5.0/examples#examples) |
| [v0.4.0](https://github.com/tektoncd/pipeline/releases/tag/v0.4.0) | [Docs @ v0.4.0](https://github.com/tektoncd/pipeline/tree/v0.4.0/docs#tekton-pipelines) | [Examples @ v0.4.0](https://github.com/tektoncd/pipeline/tree/v0.4.0/examples#examples) |
| [v0.3.1](https://github.com/tektoncd/pipeline/releases/tag/v0.3.1) | [Docs @ v0.3.1](https://github.com/tektoncd/pipeline/tree/v0.3.1/docs#tekton-pipelines) | [Examples @ v0.3.1](https://github.com/tektoncd/pipeline/tree/v0.3.1/examples#examples) |
| [v0.3.0](https://github.com/tektoncd/pipeline/releases/tag/v0.3.0) | [Docs @ v0.3.0](https://github.com/tektoncd/pipeline/tree/v0.3.0/docs#tekton-pipelines) | [Examples @ v0.3.0](https://github.com/tektoncd/pipeline/tree/v0.3.0/examples#examples) |
| [v0.2.0](https://github.com/tektoncd/pipeline/releases/tag/v0.2.0) | [Docs @ v0.2.0](https://github.com/tektoncd/pipeline/tree/v0.2.0/docs#tekton-pipelines) | [Examples @ v0.2.0](https://github.com/tektoncd/pipeline/tree/v0.2.0/examples#examples) |
| [v0.1.0](https://github.com/tektoncd/pipeline/releases/tag/v0.1.0) | [Docs @ v0.1.0](https://github.com/tektoncd/pipeline/tree/v0.1.0/docs#tekton-pipelines) | [Examples @ v0.1.0](https://github.com/tektoncd/pipeline/tree/v0.1.0/examples#examples) |

_See [our API compatibility policy](api_compatibility_policy.md) for info on the
stability level of the API._

_See [our Deprecations table](docs/deprecations.md) for features that have been
deprecated and the earliest date they'll be removed._

## Migrating

### v1alpha1 to v1beta1

In the move from v1alpha1 to v1beta1 several spec fields and Tekton
CRDs were updated or removed .

For users migrating their Tasks and Pipelines from v1alpha1 to v1beta1, check
out [the spec changes and migration paths](./docs/migrating-v1alpha1-to-v1beta1.md).

## Want to contribute

We are so excited to have you!

- See [CONTRIBUTING.md](CONTRIBUTING.md) for an overview of our processes
- See [DEVELOPMENT.md](DEVELOPMENT.md) for how to get started
- [Deep dive](./docs/developers/README.md) into demystifying the inner workings
  (advanced reading material)
- Look at our
  [good first issues](https://github.com/tektoncd/pipeline/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
  and our
  [help wanted issues](https://github.com/tektoncd/pipeline/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22)
