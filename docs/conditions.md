<!--
---
linkTitle: "Conditions"
weight: 2100
---
-->
# Conditions

**Note:** `Conditions` are deprecated, use [`when` expressions](pipelines.md#guard-task-execution-using-when-expressions) instead. 

- [Overview](#overview)
- [Configuring a `Condition`](#configuring-a-condition)
  - [Specifying the condition `check`](#specifying-the-condition-check)
  - [Specifying `Parameters`](#specifying-parameters)
  - [Specifying `Resources`](#specifying-resources)
- [Code examples](#code-examples)

## Overview

A `Condition` resource in Tekton allows you to conditionalize the execution of `Tasks` within a `Pipeline`.
You define each `Condition` within your [`PipelineRun` definition](pipelineruns.md) and then conditionalize
each desired [`Task`](tasks.md) in the corresponding `Pipeline` definition. 

The `Condition` resource runs its own container image that executes the logic that evaluates your chosen condition.
This container runs to completion and must return an exit code value of `0` for the `check` to be successful; otherwise
the conditionalized `Task` as well as its `Task` dependencies (defined via `runAfter`) and `Resource` dependencies
(such as results) do not execute.

**Note:** [Labels](labels.md) and annotations specified in the `Condition's` metadata are automatically
propagated to the `Pod`.

## Configuring a `Condition`

A `Condition` definition supports the following fields:

- Required:
  - [`apiVersion`][kubernetes-overview] - Specifies the API version, for example
    `tekton.dev/v1alpha1`.
  - [`kind`][kubernetes-overview] - Identifies this resource object as a `Condition` object.
  - [`metadata`][kubernetes-overview] - Specifies metadata that uniquely identifies this
    `Condition` object. For example, a `name`.
  - [`spec`][kubernetes-overview] - Specifies the configuration information for
    this `Condition` resource object. This must include:
    - [`check`](#check) - Specifies a container that you want to run for evaluating this `Condition`.
    - [`description`](#description) - Provides a meaningful description of this `Condition` object.

[kubernetes-overview]:
  https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/#required-fields

### Specifying the condition `check` 

The `check` field (required) specifies a single piece of evaluation logic that you want to run before the
corresponding `Task` in your `Pipeline` can execute. This field must specify a [`Step`](./tasks.md#steps). 

### Specifying `Parameters`

You can specify parameters to pass to the `Condition's` evaluation logic at run time. 
Sub-fields within the `check` field can access these parameter values using Tekton's templating
syntax as follows:

```yaml
spec:
  parameters:
    - name: image
      default: ubuntu
  check:
    image: $(params.image)
```

Parameter names:
- Must only contain alphanumeric characters, hyphens (`-`), and underscores (`_`).
- Must begin with a letter or an underscore (`_`).

For example, `fooIs-Bar_` is a valid parameter name, but `barIsBa$` or `0banana` are not.

Each declared parameter has a `type` field, which can be set to either `array` or `string`, and
defaults to `string` if you don't specify a value. The `description` and `default` fields for a
`Parameter` are optional. The `array` type is useful in situations such as checking that a pushed
branch name doesn't collide with any of the specified protected branch names.

### Specifying `Resources`

You can specify input [`PipelineResources`](resources.md) in your `Condition` definition to 
provide the `Condition's` container step with data or context necessary to run the evaluation logic.

`Resources` in `Conditions` behave the same way as in `Tasks`:
- You can access them via [variable substitution](resources.md#variable-substitution).
- You can use the `targetPath` field to [specify a mount point](resources.md#controlling-where-resources-are-mounted).

### Adding a `description`

The `description` field (optional) allows you to specify a meaningful description for your `Condition`.

## Code examples

For a better understanding of `Conditions`, study [our code examples](https://github.com/tektoncd/pipeline/tree/main/examples).

---

Except as otherwise noted, the content of this page is licensed under the
[Creative Commons Attribution 4.0 License](https://creativecommons.org/licenses/by/4.0/),
and code samples are licensed under the
[Apache 2.0 License](https://www.apache.org/licenses/LICENSE-2.0).
