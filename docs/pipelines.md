<!--
---
linkTitle: "Pipelines"
weight: 400
---
-->
# Pipelines

- [Overview](#pipelines)
- [Configuring a `Pipeline`](#configuring-a-pipeline)
  - [Specifying `Resources`](#specifying-resources)
  - [Specifying `Workspaces`](#specifying-workspaces)
  - [Specifying `Parameters`](#specifying-parameters)
  - [Adding `Tasks` to the `Pipeline`](#adding-tasks-to-the-pipeline)
    - [Tekton Bundles](#tekton-bundles)
    - [Using the `from` parameter](#using-the-from-parameter)
    - [Using the `runAfter` parameter](#using-the-runafter-parameter)
    - [Using the `retries` parameter](#using-the-retries-parameter)
    - [Guard `Task` execution using `when` expressions](#guard-task-execution-using-when-expressions)
    - [Guard `Task` execution using `Conditions`](#guard-task-execution-using-conditions)
    - [Configuring the failure timeout](#configuring-the-failure-timeout)
  - [Using variable substitution](#using-variable-substitution)
    - [Using the `retries` and `retry-count` variable substitutions](#using-the-retries-and-retry-count-variable-substitutions)
  - [Using `Results`](#using-results)
    - [Passing one Task's `Results` into the `Parameters` or `when` expressions of another](#passing-one-tasks-results-into-the-parameters-or-when-expressions-of-another)
    - [Emitting `Results` from a `Pipeline`](#emitting-results-from-a-pipeline)
  - [Configuring the `Task` execution order](#configuring-the-task-execution-order)
  - [Adding a description](#adding-a-description)
  - [Adding `Finally` to the `Pipeline`](#adding-finally-to-the-pipeline)
    - [Specifying `Workspaces` in Final Tasks](#specifying-workspaces-in-final-tasks)
    - [Specifying `Parameters` in Final Tasks](#specifying-parameters-in-final-tasks)
    - [Consuming `Task` execution results in `finally`](#consuming-task-execution-results-in-finally)
    - [`PipelineRun` Status with `finally`](#pipelinerun-status-with-finally)
    - [Using Execution `Status` of `pipelineTask`](#using-execution-status-of-pipelinetask)
    - [Using Aggregate Execution `Status` of All `Tasks`](#using-aggregate-execution-status-of-all-tasks)
    - [Guard `Finally Task` execution using `when` expressions](#guard-finally-task-execution-using-when-expressions)
      - [`when` expressions using `Parameters` in `Finally Tasks`](#when-expressions-using-parameters-in-finally-tasks)
      - [`when` expressions using `Results` in `Finally Tasks`](#when-expressions-using-results-in-finally-tasks)
      - [`when` expressions using `Execution Status` of `PipelineTask` in `Finally Tasks`](#when-expressions-using-execution-status-of-pipelinetask-in-finally-tasks)
      - [`when` expressions using `Aggregate Execution Status` of `Tasks` in `Finally Tasks`](#when-expressions-using-aggregate-execution-status-of-tasks-in-finally-tasks)
    - [Known Limitations](#known-limitations)
      - [Specifying `Resources` in Final Tasks](#specifying-resources-in-final-tasks)
      - [Cannot configure the Final Task execution order](#cannot-configure-the-final-task-execution-order)
      - [Cannot specify execution `Conditions` in Final Tasks](#cannot-specify-execution-conditions-in-final-tasks)
      - [Cannot configure `Pipeline` result with `finally`](#cannot-configure-pipeline-result-with-finally)
  - [Using Custom Tasks](#using-custom-tasks)
    - [Specifying the target Custom Task](#specifying-the-target-custom-task)
    - [Specifying parameters](#specifying-parameters-1)
    - [Specifying workspaces](#specifying-workspaces-1)
    - [Using `Results`](#using-results-1)
    - [Limitations](#limitations)
  - [Code examples](#code-examples)

## Overview

A `Pipeline` is a collection of `Tasks` that you define and arrange in a specific order
of execution as part of your continuous integration flow. Each `Task` in a `Pipeline`
executes as a `Pod` on your Kubernetes cluster. You can configure various execution
conditions to fit your business needs.

## Configuring a `Pipeline`

A `Pipeline` definition supports the following fields:

- Required:
  - [`apiVersion`][kubernetes-overview] - Specifies the API version, for example
    `tekton.dev/v1beta1`.
  - [`kind`][kubernetes-overview] - Identifies this resource object as a `Pipeline` object.
  - [`metadata`][kubernetes-overview] - Specifies metadata that uniquely identifies the
    `Pipeline` object. For example, a `name`.
  - [`spec`][kubernetes-overview] - Specifies the configuration information for
    this `Pipeline` object. This must include:
      - [`tasks`](#adding-tasks-to-the-pipeline) - Specifies the `Tasks` that comprise the `Pipeline`
        and the details of their execution.
- Optional:
  - [`resources`](#specifying-resources) - **alpha only** Specifies
    [`PipelineResources`](resources.md) needed or created by the `Tasks` comprising the `Pipeline`.
  - [`tasks`](#adding-tasks-to-the-pipeline):
      - `resources.inputs` / `resource.outputs`
        - [`from`](#using-the-from-parameter) - Indicates the data for a [`PipelineResource`](resources.md)
          originates from the output of a previous `Task`.
      - [`runAfter`](#using-the-runafter-parameter) - Indicates that a `Task`
        should execute after one or more other `Tasks` without output linking.
      - [`retries`](#using-the-retries-parameter) - Specifies the number of times to retry the
        execution of a `Task` after a failure. Does not apply to execution cancellations.
      - [`conditions`](#guard-task-execution-using-conditions) - Specifies `Conditions` that only allow a `Task`
        to execute if they successfully evaluate.
      - [`timeout`](#configuring-the-failure-timeout) - Specifies the timeout before a `Task` fails.
  - [`results`](#configuring-execution-results-at-the-pipeline-level) - Specifies the location to which
    the `Pipeline` emits its execution results.
  - [`description`](#adding-a-description) - Holds an informative description of the `Pipeline` object.
  - [`finally`](#adding-finally-to-the-pipeline) - Specifies one or more `Tasks`
    to be executed in parallel after all other tasks have completed.

[kubernetes-overview]:
  https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/#required-fields

## Specifying `Resources`

A `Pipeline` requires [`PipelineResources`](resources.md) to provide inputs and store outputs
for the `Tasks` that comprise it. You can declare those in the `resources` field in the `spec`
section of the `Pipeline` definition. Each entry requires a unique `name` and a `type`. For example:

```yaml
spec:
  resources:
    - name: my-repo
      type: git
    - name: my-image
      type: image
```

## Specifying `Workspaces`

`Workspaces` allow you to specify one or more volumes that each `Task` in the `Pipeline`
requires during execution. You specify one or more `Workspaces` in the `workspaces` field.
For example:

```yaml
spec:
  workspaces:
    - name: pipeline-ws1 # The name of the workspace in the Pipeline
  tasks:
    - name: use-ws-from-pipeline
      taskRef:
        name: gen-code # gen-code expects a workspace with name "output"
      workspaces:
        - name: output
          workspace: pipeline-ws1
    - name: use-ws-again
      taskRef:
        name: commit # commit expects a workspace with name "src"
      runAfter:
        - use-ws-from-pipeline # important: use-ws-from-pipeline writes to the workspace first
      workspaces:
        - name: src
          workspace: pipeline-ws1
```

For more information, see:
- [Using `Workspaces` in `Pipelines`](workspaces.md#using-workspaces-in-pipelines)
- The [`Workspaces` in a `PipelineRun`](../examples/v1beta1/pipelineruns/workspaces.yaml) code example
- The [variables available in a `PipelineRun`](variables.md#variables-available-in-a-pipeline), including `workspaces.<name>.bound`.

## Specifying `Parameters`

You can specify global parameters, such as compilation flags or artifact names, that you want to supply
to the `Pipeline` at execution time. `Parameters` are passed to the `Pipeline` from its corresponding
`PipelineRun` and can replace template values specified within each `Task` in the `Pipeline`.

Parameter names:
- Must only contain alphanumeric characters, hyphens (`-`), and underscores (`_`).
- Must begin with a letter or an underscore (`_`).

For example, `fooIs-Bar_` is a valid parameter name, but `barIsBa$` or `0banana` are not.

Each declared parameter has a `type` field, which can be set to either `array` or `string`.
`array` is useful in cases where the number of compilation flags being supplied to the `Pipeline`
varies throughout its execution. If no value is specified, the `type` field defaults to `string`.
When the actual parameter value is supplied, its parsed type is validated against the `type` field.
The `description` and `default` fields for a `Parameter` are optional.

The following example illustrates the use of `Parameters` in a `Pipeline`.

The following `Pipeline` declares two input parameters :

- `context` which passes its value (a string) to the `Task` to set the value of the `pathToContext` parameter within the `Task`.
- `flags` which passes its value (an array) to the `Task` to set the value of
  the `flags` parameter within the `Task`. The `flags` parameter within the
`Task` **must** also be an array.
If you specify a value for the `default` field and invoke this `Pipeline` in a `PipelineRun`
without specifying a value for `context`, that value will be used.

**Note:** Input parameter values can be used as variables throughout the `Pipeline`
by using [variable substitution](variables.md#variables-available-in-a-pipeline).

```yaml
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: pipeline-with-parameters
spec:
  params:
    - name: context
      type: string
      description: Path to context
      default: /some/where/or/other
    - name: flags
      type: array
      description: List of flags
  tasks:
    - name: build-skaffold-web
      taskRef:
        name: build-push
      params:
        - name: pathToDockerFile
          value: Dockerfile
        - name: pathToContext
          value: "$(params.context)"
        - name: flags
          value: ["$(params.flags[*])"]
```

The following `PipelineRun` supplies a value for `context`:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  name: pipelinerun-with-parameters
spec:
  pipelineRef:
    name: pipeline-with-parameters
  params:
    - name: "context"
      value: "/workspace/examples/microservices/leeroy-web"
    - name: "flags"
      value:
        - "foo"
        - "bar"
```

## Adding `Tasks` to the `Pipeline`

 Your `Pipeline` definition must reference at least one [`Task`](tasks.md).
Each `Task` within a `Pipeline` must have a [valid](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names)
`name` and a `taskRef`. For example:

```yaml
tasks:
  - name: build-the-image
    taskRef:
      name: build-push
```

You can use [`PipelineResources`](#specifying-resources) as inputs and outputs for `Tasks`
in the `Pipeline`. For example:

```yaml
spec:
  tasks:
    - name: build-the-image
      taskRef:
        name: build-push
      resources:
        inputs:
          - name: workspace
            resource: my-repo
        outputs:
          - name: image
            resource: my-image
```

You can also provide [`Parameters`](tasks.md#specifying-parameters):

```yaml
spec:
  tasks:
    - name: build-skaffold-web
      taskRef:
        name: build-push
      params:
        - name: pathToDockerFile
          value: Dockerfile
        - name: pathToContext
          value: /workspace/examples/microservices/leeroy-web
```

### Tekton Bundles

**Note: This is only allowed if `enable-tekton-oci-bundles` is set to
`"true"` in the `feature-flags` configmap, see [`install.md`](./install.md#customizing-the-pipelines-controller-behavior)**

You may also specify your `Task` reference using a `Tekton Bundle`. A `Tekton Bundle` is an OCI artifact that
contains Tekton resources like `Tasks` which can be referenced within a `taskRef`.

 ```yaml
 spec:
   tasks:
     - name: hello-world
       taskRef:
         name: echo-task
         bundle: docker.com/myrepo/mycatalog
 ```

Here, the `bundle` field is the full reference url to the artifact. The name is the
`metadata.name` field of the `Task`.

You may also specify a `tag` as you would with a Docker image which will give you a fixed,
repeatable reference to a `Task`.

 ```yaml
 spec:
   tasks:
     - name: hello-world
       taskRef:
         name: echo-task
         bundle: docker.com/myrepo/mycatalog:v1.0.1
 ```

You may also specify a fixed digest instead of a tag.

 ```yaml
 spec:
   tasks:
     - name: hello-world
       taskRef:
         name: echo-task
         bundle: docker.io/myrepo/mycatalog@sha256:abc123
 ```

Any of the above options will fetch the image using the `ImagePullSecrets` attached to the
`ServiceAccount` specified in the `PipelineRun`.
See the [Service Account](pipelineruns.md#specifying-custom-serviceaccount-credentials) section
for details on how to configure a `ServiceAccount` on a `PipelineRun`. The `PipelineRun` will then
run that `Task` without registering it in the cluster allowing multiple versions of the same named
`Task` to be run at once.

`Tekton Bundles` may be constructed with any toolsets that produce valid OCI image artifacts
so long as the artifact adheres to the [contract](tekton-bundle-contracts.md).

### Using the `from` parameter

If a `Task` in your `Pipeline` needs to use the output of a previous `Task`
as its input, use the optional `from` parameter to specify a list of `Tasks`
that must execute **before** the `Task` that requires their outputs as its
input. When your target `Task` executes, only the version of the desired
`PipelineResource` produced by the last `Task` in this list is used. The
`name` of this output `PipelineResource` output must match the `name` of the
input `PipelineResource` specified in the `Task` that ingests it.

In the example below, the `deploy-app` `Task` ingests the output of the `build-app`
`Task` named `my-image` as its input.  Therefore, the `build-app` `Task` will
execute before the `deploy-app` `Task` regardless of the order in which those
`Tasks` are declared in the `Pipeline`.

```yaml
- name: build-app
  taskRef:
    name: build-push
  resources:
    outputs:
      - name: image
        resource: my-image
- name: deploy-app
  taskRef:
    name: deploy-kubectl
  resources:
    inputs:
      - name: image
        resource: my-image
        from:
          - build-app
```

### Using the `runAfter` parameter

If you need your `Tasks` to execute in a specific order within the `Pipeline`
but they don't have resource dependencies that require the `from` parameter,
use the `runAfter` parameter to indicate that a `Task` must execute after
one or more other `Tasks`.

In the example below, we want to test the code before we build it. Since there
is no output from the `test-app` `Task`, the `build-app` `Task` uses `runAfter`
to indicate that `test-app` must run before it, regardless of the order in which
they are referenced in the `Pipeline` definition.

```yaml
- name: test-app
  taskRef:
    name: make-test
  resources:
    inputs:
      - name: workspace
        resource: my-repo
- name: build-app
  taskRef:
    name: kaniko-build
  runAfter:
    - test-app
  resources:
    inputs:
      - name: workspace
        resource: my-repo
```

### Using the `retries` parameter

For each `Task` in the `Pipeline`, you can specify the number of times Tekton
should retry its execution when it fails. When a `Task` fails, the corresponding
`TaskRun` sets its `Succeeded` `Condition` to `False`. The `retries` parameter
instructs Tekton to retry executing the `Task` when this happens.

If you expect a `Task` to encounter problems during execution (for example,
you know that there will be issues with network connectivity or missing
dependencies), set its `retries` parameter to a suitable value greater than 0.
If you don't explicitly specify a value, Tekton does not attempt to execute
the failed `Task` again.

In the example below, the execution of the `build-the-image` `Task` will be
retried once after a failure; if the retried execution fails, too, the `Task`
execution fails as a whole.

```yaml
tasks:
  - name: build-the-image
    retries: 1
    taskRef:
      name: build-push
```

### Guard `Task` execution using `when` expressions

To run a `Task` only when certain conditions are met, it is possible to _guard_ task execution using the `when` field. The `when` field allows you to list a series of references to `when` expressions.

The components of `when` expressions are `input`, `operator` and `values`:
- `input` is the input for the `when` expression which can be static inputs or variables ([`Parameters`](#specifying-parameters) or [`Results`](#using-results)). If the `input` is not provided, it defaults to an empty string.
- `operator` represents an `input`'s relationship to a set of `values`. A valid `operator` must be provided, which can be either `in` or `notin`.
- `values` is an array of string values. The `values` array must be provided and be non-empty. It can contain static values or variables ([`Parameters`](#specifying-parameters), [`Results`](#using-results) or [a Workspaces's `bound` state](#specifying-workspaces)).

The [`Parameters`](#specifying-parameters) are read from the `Pipeline` and [`Results`](#using-results) are read directly from previous [`Tasks`](#adding-tasks-to-the-pipeline). Using [`Results`](#using-results) in a `when` expression in a guarded `Task` introduces a resource dependency on the previous `Task` that produced the `Result`.

The declared `when` expressions are evaluated before the `Task` is run. If all the `when` expressions evaluate to `True`, the `Task` is run. If any of the `when` expressions evaluate to `False`, the `Task` is not run and the `Task` is listed in the [`Skipped Tasks` section of the `PipelineRunStatus`](pipelineruns.md#monitoring-execution-status).

In these examples, `first-create-file` task will only be executed if the `path` parameter is `README.md`, `echo-file-exists` task will only be executed if the `exists` result from `check-file` task is `yes` and `run-lint` task will only be executed if the `lint-config` optional workspace has been provided by a PipelineRun.

```yaml
tasks:
  - name: first-create-file
    when:
      - input: "$(params.path)"
        operator: in
        values: ["README.md"]
    taskRef:
      name: first-create-file
---
tasks:
  - name: echo-file-exists
    when:
      - input: "$(tasks.check-file.results.exists)"
        operator: in
        values: ["yes"]
    taskRef:
      name: echo-file-exists
---
tasks:
  - name: run-lint
    when:
      - input: "$(workspaces.lint-config.bound)"
        operator: in
        values: ["true"]
    taskRef:
      name: lint-source
```

For an end-to-end example, see [PipelineRun with `when` expressions](../examples/v1beta1/pipelineruns/pipelinerun-with-when-expressions.yaml).

When `when` expressions are specified in a `Task`, [`Conditions`](#guard-task-execution-using-conditions) should not be specified in the same `Task`. The `Pipeline` will be rejected as invalid if both `when` expressions and `Conditions` are included.

There are a lot of scenarios where `when` expressions can be really useful. Some of these are:
- Checking if the name of a git branch matches
- Checking if the `Result` of a previous `Task` is as expected
- Checking if a git file has changed in the previous commits
- Checking if an image exists in the registry
- Checking if the name of a CI job matches
- Checking if an optional Workspace has been provided

### Guard `Task` execution using `Conditions`

**Note:** `Conditions` are [deprecated](./deprecations.md), use [`when` expressions](#guard-task-execution-using-when-expressions) instead.

To run a `Task` only when certain conditions are met, it is possible to _guard_ task execution using
the `conditions` field. The `conditions` field allows you to list a series of references to
[`Condition`](./conditions.md) resources. The declared `Conditions` are run before the `Task` is run.
If all of the conditions successfully evaluate, the `Task` is run. If any of the conditions fails,
the `Task` is not run and the `TaskRun` status field `ConditionSucceeded` is set to `False` with the
reason set to `ConditionCheckFailed`.

In this example, `is-master-branch` refers to a [Condition](conditions.md) resource. The `deploy`
task will only be executed if the condition successfully evaluates.

```yaml
tasks:
  - name: deploy-if-branch-is-master
    conditions:
      - conditionRef: is-master-branch
        params:
          - name: branch-name
            value: my-value
    taskRef:
      name: deploy
```

Unlike regular task failures, condition failures do not automatically fail the entire `PipelineRun` --
other tasks that are **not dependent** on the `Task` (via `from` or `runAfter`) are still run.

In this example, `(task C)` has a `condition` set to _guard_ its execution. If the condition
is **not** successfully evaluated, task `(task D)` will not be run, but all other tasks in the pipeline
that not depend on `(task C)` will be executed and the `PipelineRun` will successfully complete.

  ```
         (task B) — (task E)
       /
   (task A)
       \
         (guarded task C) — (task D)
  ```

Resources in conditions can also use the [`from`](#using-the-from-parameter) field to indicate that they
expect the output of a previous task as input. As with regular Pipeline Tasks, using `from`
implies ordering --  if task has a condition that takes in an output resource from
another task, the task producing the output resource will run first:

```yaml
tasks:
  - name: first-create-file
    taskRef:
      name: create-file
    resources:
      outputs:
        - name: workspace
          resource: source-repo
  - name: then-check
    conditions:
      - conditionRef: "file-exists"
        resources:
          - name: workspace
            resource: source-repo
            from: [first-create-file]
    taskRef:
      name: echo-hello
```

### Configuring the failure timeout

You can use the `Timeout` field in the `Task` spec within the `Pipeline` to set the timeout
of the `TaskRun` that executes that `Task` within the `PipelineRun` that executes your `Pipeline.`
The `Timeout` value is a `duration` conforming to Go's [`ParseDuration`](https://golang.org/pkg/time/#ParseDuration)
format. For example, valid values are `1h30m`, `1h`, `1m`, and `60s`.

**Note:** If you do not specify a `Timeout` value, Tekton instead honors the timeout for the [`PipelineRun`](pipelineruns.md#configuring-a-pipelinerun).

In the example below, the `build-the-image` `Task` is configured to time out after 90 seconds:

```yaml
spec:
  tasks:
    - name: build-the-image
      taskRef:
        name: build-push
      timeout: "0h1m30s"
```

## Using variable substitution

Tekton provides variables to inject values into the contents of certain fields.
The values you can inject come from a range of sources including other fields
in the Pipeline, context-sensitive information that Tekton provides, and runtime
information received from a PipelineRun.

The mechanism of variable substitution is quite simple - string replacement is
performed by the Tekton Controller when a PipelineRun is executed.

See the [complete list of variable substitutions for Pipelines](./variables.md#variables-available-in-a-pipeline)
and the [list of fields that accept substitutions](./variables.md#fields-that-accept-variable-substitutions).

### Using the `retries` and `retry-count` variable substitutions

Tekton supports variable substitution for the [`retries`](#using-the-retries-parameter)
parameter of `PipelineTask`. Variables like `context.pipelineTask.retries` and
`context.task.retry-count` can be added to the parameters of a `PipelineTask`.
`context.pipelineTask.retries` will be replaced by `retries` of the `PipelineTask`, while
`context.task.retry-count` will be replaced by current retry number of the `PipelineTask`.

```yaml
params:
- name: pipelineTask-retries
  value: "$(context.pipelineTask.retries)"
taskSpec:
  params:
  - name: pipelineTask-retries
  steps:
  - image: ubuntu
    name: print-if-retries-exhausted
    script: |
      if [ "$(context.task.retry-count)" == "$(params.pipelineTask-retries)" ]
      then
        echo "This is the last retry."
      fi
      exit 1
```

**Note:** Every `PipelineTask` can only access its own `retries` and `retry-count`. These
values aren't accessible for other `PipelineTask`s.

## Using `Results`

Tasks can emit [`Results`](tasks.md#emitting-results) when they execute. A Pipeline can use these
`Results` for two different purposes:

1. A Pipeline can pass the `Result` of a `Task` into the `Parameters` or `when` expressions of another.
2. A Pipeline can itself emit `Results` and include data from the `Results` of its Tasks.

### Passing one Task's `Results` into the `Parameters` or `when` expressions of another

Sharing `Results` between `Tasks` in a `Pipeline` happens via
[variable substitution](variables.md#variables-available-in-a-pipeline) - one `Task` emits
a `Result` and another receives it as a `Parameter` with a variable such as
`$(tasks.<task-name>.results.<result-name>)`.

When one `Task` receives the `Results` of another, there is a dependency created between those
two `Tasks`. In order for the receiving `Task` to get data from another `Task's` `Result`,
the `Task` producing the `Result` must run first. Tekton enforces this `Task` ordering
by ensuring that the `Task` emitting the `Result` executes before any `Task` that uses it.

In the snippet below, a param is provided its value from the `commit` `Result` emitted by the
`checkout-source` `Task`. Tekton will make sure that the `checkout-source` `Task` runs
before this one.

```yaml
params:
  - name: foo
    value: "$(tasks.checkout-source.results.commit)"
```

**Note:** If `checkout-source` exits successfully without initializing `commit` `Result`,
the receiving `Task` fails and causes the `Pipeline` to fail with `InvalidTaskResultReference`:

```
unable to find result referenced by param 'foo' in 'task';: Could not find result with name 'commit' for task run 'checkout-source'
```

In the snippet below, a `when` expression is provided its value from the `exists` `Result` emitted by the
`check-file` `Task`. Tekton will make sure that the `check-file` `Task` runs before this one.

```yaml
when:
  - input: "$(tasks.check-file.results.exists)"
    operator: in
    values: ["yes"]
```

For an end-to-end example, see [`Task` `Results` in a `PipelineRun`](../examples/v1beta1/pipelineruns/task_results_example.yaml).

### Emitting `Results` from a `Pipeline`

A `Pipeline` can emit `Results` of its own for a variety of reasons - an external
system may need to read them when the `Pipeline` is complete, they might summarise
the most important `Results` from the `Pipeline's` `Tasks`, or they might simply
be used to expose non-critical messages generated during the execution of the `Pipeline`.

A `Pipeline's` `Results` can be composed of one or many `Task` `Results` emitted during
the course of the `Pipeline's` execution. A `Pipeline` `Result` can refer to its `Tasks'`
`Results` using a variable of the form `$(tasks.<task-name>.results.<result-name>)`.

After a `Pipeline` has executed the `PipelineRun` will be populated with the `Results`
emitted by the `Pipeline`. These will be written to the `PipelineRun's`
`status.pipelineResults` field.

In the example below, the `Pipeline` specifies a `results` entry with the name `sum` that
references the `outputValue` `Result` emitted by the `calculate-sum` `Task`.

```yaml
results:
  - name: sum
    description: the sum of all three operands
    value: $(tasks.calculate-sum.results.outputValue)
```

For an end-to-end example, see [`Results` in a `PipelineRun`](../examples/v1beta1/pipelineruns/pipelinerun-results.yaml).

A `Pipeline Result` is not emitted if any of the following are true:
- A `PipelineTask` referenced by the `Pipeline Result` failed. The `PipelineRun` will also
have failed.
- A `PipelineTask` referenced by the `Pipeline Result` was skipped.
- A `PipelineTask` referenced by the `Pipeline Result` didn't emit the referenced `Task Result`. This
should be considered a bug in the `Task` and [may fail a `PipelineTask` in future](https://github.com/tektoncd/pipeline/issues/3497).
- The `Pipeline Result` uses a variable that doesn't point to an actual `PipelineTask`. This will
result in an `InvalidTaskResultReference` validation error during `PipelineRun` execution.
- The `Pipeline Result` uses a variable that doesn't point to an actual result in a `PipelineTask`.
This will cause an `InvalidTaskResultReference` validation error during `PipelineRun` execution.

**Note:** Since a `Pipeline Result` can contain references to multiple `Task Results`, if any of those
`Task Result` references are invalid the entire `Pipeline Result` is not emitted.

## Configuring the `Task` execution order

You can connect `Tasks` in a `Pipeline` so that they execute in a Directed Acyclic Graph (DAG).
Each `Task` in the `Pipeline` becomes a node on the graph that can be connected with an edge
so that one will run before another and the execution of the `Pipeline` progresses to completion
without getting stuck in an infinite loop.

This is done using:

- [`from`](#using-the-from-parameter) clauses on the [`PipelineResources`](resources.md) used by each `Task`
- [`runAfter`](#using-the-runafter-parameter) clauses on the corresponding `Tasks`
- By linking the [`results`](#configuring-execution-results-at-the-pipeline-level) of one `Task` to the params of another

For example, the `Pipeline` defined as follows

```yaml
- name: lint-repo
  taskRef:
    name: pylint
  resources:
    inputs:
      - name: workspace
        resource: my-repo
- name: test-app
  taskRef:
    name: make-test
  resources:
    inputs:
      - name: workspace
        resource: my-repo
- name: build-app
  taskRef:
    name: kaniko-build-app
  runAfter:
    - test-app
  resources:
    inputs:
      - name: workspace
        resource: my-repo
    outputs:
      - name: image
        resource: my-app-image
- name: build-frontend
  taskRef:
    name: kaniko-build-frontend
  runAfter:
    - test-app
  resources:
    inputs:
      - name: workspace
        resource: my-repo
    outputs:
      - name: image
        resource: my-frontend-image
- name: deploy-all
  taskRef:
    name: deploy-kubectl
  resources:
    inputs:
      - name: my-app-image
        resource: my-app-image
        from:
          - build-app
      - name: my-frontend-image
        resource: my-frontend-image
        from:
          - build-frontend
```

executes according to the following graph:

```none
        |            |
        v            v
     test-app    lint-repo
    /        \
   v          v
build-app  build-frontend
   \          /
    v        v
    deploy-all
```

In particular:

1. The `lint-repo` and `test-app` `Tasks` have no `from` or `runAfter` clauses
   and start executing simultaneously.
2. Once `test-app` completes, both `build-app` and `build-frontend` start
   executing simultaneously since they both `runAfter` the `test-app` `Task`.
3. The `deploy-all` `Task` executes once both `build-app` and `build-frontend`
   complete, since it ingests `PipelineResources` from both.
4. The entire `Pipeline` completes execution once both `lint-repo` and `deploy-all`
   complete execution.

## Adding a description

The `description` field is an optional field and can be used to provide description of the `Pipeline`.

## Adding `Finally` to the `Pipeline`

You can specify a list of one or more final tasks under `finally` section. Final tasks are guaranteed to be executed
in parallel after all `PipelineTasks` under `tasks` have completed regardless of success or error. Final tasks are very
similar to `PipelineTasks` under `tasks` section and follow the same syntax. Each final task must have a
[valid](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names) `name` and a [taskRef or
taskSpec](taskruns.md#specifying-the-target-task). For example:

```yaml
spec:
  tasks:
    - name: tests
      taskRef:
        name: integration-test
  finally:
    - name: cleanup-test
      taskRef:
        name: cleanup
```

### Specifying `Workspaces` in Final Tasks

Finally tasks can specify [workspaces](workspaces.md) which `PipelineTasks` might have utilized
e.g. a mount point for credentials held in Secrets. To support that requirement, you can specify one or more
`Workspaces` in the `workspaces` field for the final tasks similar to `tasks`.

```yaml
spec:
  resources:
    - name: app-git
      type: git
  workspaces:
    - name: shared-workspace
  tasks:
    - name: clone-app-source
      taskRef:
        name: clone-app-repo-to-workspace
      workspaces:
        - name: shared-workspace
          workspace: shared-workspace
      resources:
        inputs:
          - name: app-git
            resource: app-git
  finally:
    - name: cleanup-workspace
      taskRef:
        name: cleanup-workspace
      workspaces:
        - name: shared-workspace
          workspace: shared-workspace
```

### Specifying `Parameters` in Final Tasks

Similar to `tasks`, you can specify [`Parameters`](tasks.md#specifying-parameters) in final tasks:

```yaml
spec:
  tasks:
    - name: tests
      taskRef:
        name: integration-test
  finally:
    - name: report-results
      taskRef:
        name: report-results
      params:
        - name: url
          value: "someURL"
```

### Consuming `Task` execution results in `finally`

Final tasks can be configured to consume `Results` of `PipelineTask` from the `tasks` section:

```yaml
spec:
  tasks:
    - name: clone-app-repo
      taskRef:
        name: git-clone
  finally:
    - name: discover-git-commit
      params:
        - name: commit
          value: $(tasks.clone-app-repo.results.commit)
```
**Note:** The scheduling of such final task does not change, it will still be executed in parallel with other
final tasks after all non-final tasks are done.

The controller resolves task results before executing the finally task `discover-git-commit`. If the task
`clone-app-repo` failed or skipped with [when expression](#guard-task-execution-using-when-expressions) resulting in
uninitialized task result `commit`, the finally Task `discover-git-commit` will be included in the list of
`skippedTasks` and continues executing rest of the final tasks. The pipeline exits with `completion` instead of
`success` if a finally task is added to the list of `skippedTasks`.


### `PipelineRun` Status with `finally`

With `finally`, `PipelineRun` status is calculated based on `PipelineTasks` under `tasks` section and final tasks.

Without `finally`:

| `PipelineTasks` under `tasks` | `PipelineRun` status | Reason |
| ----------------------------- | -------------------- | ------ |
| all `PipelineTasks` successful | `true` | `Succeeded` |
| one or more `PipelineTasks` [skipped](conditions.md) and rest successful | `true` | `Completed` |
| single failure of `PipelineTask` | `false` | `failed` |

With `finally`:

| `PipelineTasks` under `tasks` | Final Tasks | `PipelineRun` status | Reason |
| ----------------------------- | ----------- | -------------------- | ------ |
| all `PipelineTask` successful | all final tasks successful | `true` | `Succeeded` |
| all `PipelineTask` successful | one or more failure of final tasks | `false` | `Failed` |
| one or more `PipelineTask` [skipped](conditions.md) and rest successful | all final tasks successful | `true` | `Completed` |
| one or more `PipelineTask` [skipped](conditions.md) and rest successful | one or more failure of final tasks | `false` | `Failed` |
| single failure of `PipelineTask` | all final tasks successful | `false` | `failed` |
| single failure of `PipelineTask` | one or more failure of final tasks | `false` | `failed` |

Overall, `PipelineRun` state transitioning is explained below for respective scenarios:

* All `PipelineTask` and final tasks are successful: `Started` -> `Running` -> `Succeeded`
* At least one `PipelineTask` skipped and rest successful:  `Started` -> `Running` -> `Completed`
* One `PipelineTask` failed / one or more final tasks failed: `Started` -> `Running` -> `Failed`

Please refer to the [table](pipelineruns.md#monitoring-execution-status) under Monitoring Execution Status to learn about
what kind of events are triggered based on the `Pipelinerun` status.


### Using Execution `Status` of `pipelineTask`

A `pipeline` can check the status of a specific `pipelineTask` from the `tasks` section in `finally` through the task
parameters:

```yaml
finally:
  - name: finaltask
    params:
      - name: task1Status
        value: "$(tasks.task1.status)"
    taskSpec:
      params:
        - name: task1Status
      steps:
        - image: ubuntu
          name: print-task-status
          script: |
            if [ $(params.task1Status) == "Failed" ]
            then
              echo "Task1 has failed, continue processing the failure"
            fi
```

This kind of variable can have any one of the values from the following table:

| Status | Description |
| ------- | -----------|
| `Succeeded` | `taskRun` for the `pipelineTask` completed successfully |
| `Failed` | `taskRun` for the `pipelineTask` completed with a failure or cancelled by the user |
| `None` | the `pipelineTask` has been skipped or no execution information available for the `pipelineTask` |

For an end-to-end example, see [`status` in a `PipelineRun`](../examples/v1beta1/pipelineruns/pipelinerun-task-execution-status.yaml).

### Using Aggregate Execution `Status` of All `Tasks`

A `pipeline` can check an aggregate status of all the `tasks` section in `finally` through the task parameters:

```yaml
finally:
  - name: finaltask
    params:
      - name: aggregateTasksStatus
        value: "$(tasks.status)"
    taskSpec:
      params:
        - name: aggregateTasksStatus
      steps:
        - image: ubuntu
          name: check-task-status
          script: |
            if [ $(params.aggregateTasksStatus) == "Failed" ]
            then
              echo "Looks like one or more tasks returned failure, continue processing the failure"
            fi
```

This kind of variable can have any one of the values from the following table:

| Status | Description |
| ------- | -----------|
| `Succeeded` | all `tasks` have succeeded |
| `Failed` | one ore more `tasks` failed |
| `Completed` | all `tasks` completed successfully including one or more skipped tasks |
| `None` | no aggregate execution status available (i.e. none of the above), one or more `tasks` could be pending/running/cancelled/timedout |

For an end-to-end example, see [`$(tasks.status)` usage in a `Pipeline`](../examples/v1beta1/pipelineruns/pipelinerun-task-execution-status.yaml).

### Guard `Finally Task` execution using `when` expressions

Similar to `Tasks`, `Finally Tasks` can be guarded using [`when` expressions](#guard-task-execution-using-when-expressions)
that operate on static inputs or variables. Like in `Tasks`, `when` expressions in `Finally Tasks` can operate on
`Parameters` and `Results`. Unlike in `Tasks`, `when` expressions in `Finally Tasks` can also operate on the [`Execution
Status`](#using-execution-status-of-pipelinetask) of `Tasks`.

#### `when` expressions using `Parameters` in `Finally Tasks`

`when` expressions in `Finally Tasks` can utilize `Parameters` as demonstrated using [`golang-build`](https://github.com/tektoncd/catalog/tree/main/task/golang-build/0.1)
and [`send-to-channel-slack`](https://github.com/tektoncd/catalog/tree/main/task/send-to-channel-slack/0.1) Catalog
`Tasks`:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  generateName: pipelinerun-
spec:
  pipelineSpec:
    params:
      - name: enable-notifications
        type: string
        description: a boolean indicating whether the notifications should be sent
    tasks:
      - name: golang-build
        taskRef:
          name: golang-build
      # […]
    finally:
      - name: notify-build-failure # executed only when build task fails and notifications are enabled
        when:
          - input: $(tasks.golang-build.status)
            operator: in
            values: ["Failed"]
          - input: $(params.enable-notifications)
            operator: in
            values: ["true"]
        taskRef:
          name: send-to-slack-channel
      # […]
  params:
    - name: enable-notifications
      value: true
```

#### `when` expressions using `Results` in `Finally Tasks`

`when` expressions in `Finally Tasks` can utilize `Results`, as demonstrated using [`git-clone`](https://github.com/tektoncd/catalog/tree/main/task/git-clone/0.2)
and [`github-add-comment`](https://github.com/tektoncd/catalog/tree/main/task/github-add-comment/0.2) Catalog `Tasks`:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  generateName: pipelinerun-
spec:
  pipelineSpec:
    tasks:
      - name: git-clone
        taskRef:
          name: git-clone
      - name: go-build
      # […]
    finally:
      - name: notify-commit-sha # executed only when commit sha is not the expected sha
        when:
          - input: $(tasks.git-clone.results.commit)
            operator: notin
            values: [$(params.expected-sha)]
        taskRef:
          name: github-add-comment
      # […]
  params:
    - name: expected-sha
      value: 54dd3984affab47f3018852e61a1a6f9946ecfa
```

If the `when` expressions in a `Finally Task` use `Results` from a skipped or failed non-finally `Tasks`, then the
`Finally Task` would also be skipped and be included in the list of `Skipped Tasks` in the `Status`, [similarly to when using
`Results` in other parts of the `Finally Task`](#consuming-task-execution-results-in-finally).

#### `when` expressions using `Execution Status` of `PipelineTask` in `Finally Tasks`

`when` expressions in `Finally Tasks` can utilize [`Execution Status` of `PipelineTasks`](#using-execution-status-of-pipelinetask),
as demonstrated using [`golang-build`](https://github.com/tektoncd/catalog/tree/main/task/golang-build/0.1) and
[`send-to-channel-slack`](https://github.com/tektoncd/catalog/tree/main/task/send-to-channel-slack/0.1) Catalog `Tasks`:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  generateName: pipelinerun-
spec:
  pipelineSpec:
    tasks:
      - name: golang-build
        taskRef:
          name: golang-build
      # […]
    finally:
      - name: notify-build-failure # executed only when build task fails
        when:
          - input: $(tasks.golang-build.status)
            operator: in
            values: ["Failed"]
        taskRef:
          name: send-to-slack-channel
      # […]
```

For an end-to-end example, see [PipelineRun with `when` expressions](../examples/v1beta1/pipelineruns/pipelinerun-with-when-expressions.yaml).

#### `when` expressions using `Aggregate Execution Status` of `Tasks` in `Finally Tasks`

`when` expressions in `Finally Tasks` can utilize
[`Aggregate Execution Status` of `Tasks`](#using-aggregate-execution-status-of-all-tasks) as demonstrated:

```yaml
finally:
  - name: notify-any-failure # executed only when one or more tasks fail
    when:
      - input: $(tasks.status)
        operator: in
        values: ["Failed"]
    taskRef:
      name: notify-failure
```

For an end-to-end example, see [PipelineRun with `when` expressions](../examples/v1beta1/pipelineruns/pipelinerun-with-when-expressions.yaml).

### Known Limitations

#### Specifying `Resources` in Final Tasks

Similar to `tasks`, you can use [PipelineResources](#specifying-resources) as inputs and outputs for
final tasks in the Pipeline. The only difference here is, final tasks with an input resource can not have a `from` clause
like a `PipelineTask` from `tasks` section. For example:

```yaml
spec:
  tasks:
    - name: tests
      taskRef:
        Name: integration-test
      resources:
        inputs:
          - name: source
            resource: tektoncd-pipeline-repo
        outputs:
          - name: workspace
            resource: my-repo
  finally:
    - name: clear-workspace
      taskRef:
        Name: clear-workspace
      resources:
        inputs:
          - name: workspace
            resource: my-repo
            from: #invalid
              - tests
```

#### Cannot configure the Final Task execution order

It's not possible to configure or modify the execution order of the final tasks. Unlike `Tasks` in a `Pipeline`,
all final tasks run simultaneously and start executing once all `PipelineTasks` under `tasks` have settled which means
no `runAfter` can be specified in final tasks.

#### Cannot specify execution `Conditions` in Final Tasks

`Tasks` in a `Pipeline` can be configured to run only if some conditions are satisfied using `conditions`. But the
final tasks are guaranteed to be executed after all `PipelineTasks` therefore no `conditions` can be specified in
final tasks.

#### Cannot configure `Pipeline` result with `finally`

Final tasks can emit `Results` but results emitted from the final tasks can not be configured in the
[Pipeline Results](#configuring-execution-results-at-the-pipeline-level). We are working on adding support for this
(tracked in issue [#2710](https://github.com/tektoncd/pipeline/issues/2710)).

```yaml
results:
  - name: comment-count-validate
    value: $(finally.check-count.results.comment-count-validate)
```

In this example, `pipelineResults` in `status` will exclude the name-value pair for that result `comment-count-validate`.


## Using Custom Tasks

**Note: This is only allowed if `enable-custom-tasks` is set to
`"true"` in the `feature-flags` configmap, see [`install.md`](./install.md#customizing-the-pipelines-controller-behavior)**

[Custom Tasks](https://github.com/tektoncd/community/blob/main/teps/0002-custom-tasks.md)
can implement behavior that doesn't correspond directly to running a workload in a `Pod` on the cluster.
For example, a custom task might execute some operation outside of the cluster and wait for its execution to complete.

A PipelineRun starts a custom task by creating a [`Run`](https://github.com/tektoncd/pipeline/blob/main/docs/runs.md) instead of a `TaskRun`.
In order for a custom task to execute, there must be a custom task controller running on the cluster
that is responsible for watching and updating `Run`s which reference their type.
If no such controller is running, those `Run`s will never complete and Pipelines using them will time out.

Custom tasks are an **_experimental alpha feature_** and should be expected to change
in breaking ways or even be removed.

### Specifying the target Custom Task

To specify the custom task type you want to execute, the `taskRef` field
must include the custom task's `apiVersion` and `kind` as shown below:

```yaml
spec:
  tasks:
    - name: run-custom-task
      taskRef:
        apiVersion: example.dev/v1alpha1
        kind: Example
```

This creates a `Run` of a custom task of type `Example` in the `example.dev` API group with the version `v1alpha1`.

You can also specify the `name` of a custom task resource object previously defined in the cluster.

```yaml
spec:
  tasks:
    - name: run-custom-task
      taskRef:
        apiVersion: example.dev/v1alpha1
        kind: Example
        name: myexample
```

If the `taskRef` specifies a name, the custom task controller should look up the
`Example` resource with that name and use that object to configure the execution.

If the `taskRef` does not specify a name, the custom task controller might support
some default behavior for executing unnamed tasks.

### Specifying a Custom Task Spec in-line (or embedded)

```yaml
spec:
  tasks:
    - name: run-custom-task
      taskSpec:
        apiVersion: example.dev/v1alpha1
        kind: Example
          spec:
            field1: value1
            field2: value2
```

If the custom task controller supports the in-line or embedded task spec, this will create a `Run` of a custom task of
type `Example` in the `example.dev` API group with the version `v1alpha1`.

If the `taskSpec` is not supported, the custom task controller should produce proper validation errors.

Please take a look at the
[developer guide for custom controllers supporting `taskSpec`.](runs.md#developer-guide-for-custom-controllers-supporting-spec)

`taskSpec` support for `pipelineRun` was designed and discussed in
[TEP-0061](https://github.com/tektoncd/community/blob/main/teps/0061-allow-custom-task-to-be-embedded-in-pipeline.md)

### Specifying parameters

If a custom task supports [`parameters`](tasks.md#parameters), you can use the
`params` field to specify their values:

```yaml
spec:
  tasks:
    - name: run-custom-task
      taskRef:
        apiVersion: example.dev/v1alpha1
        kind: Example
        name: myexample
      params:
        - name: foo
          value: bah
```

### Specifying workspaces

If the custom task supports it, you can provide [`Workspaces`](workspaces.md#using-workspaces-in-tasks) to share data with the custom task.

```yaml
spec:
  tasks:
    - name: run-custom-task
      taskRef:
        apiVersion: example.dev/v1alpha1
        kind: Example
        name: myexample
      workspaces:
        - name: my-workspace
```

Consult the documentation of the custom task that you are using to determine whether it supports workspaces and how to name them.

### Using `Results`

If the custom task produces results, you can reference them in a Pipeline using the normal syntax,
`$(tasks.<task-name>.results.<result-name>)`.

### Limitations

Pipelines do not support the following items with custom tasks:
* Pipeline Resources
* [`retries`](#using-the-retries-parameter)
* [`timeout`](#configuring-the-failure-timeout)
* Conditions (`Conditions` are deprecated.  Use [`when` expressions](#guard-task-execution-using-when-expressions) instead.)

## Code examples

For a better understanding of `Pipelines`, study [our code examples](https://github.com/tektoncd/pipeline/tree/main/examples).

---

Except as otherwise noted, the content of this page is licensed under the
[Creative Commons Attribution 4.0 License](https://creativecommons.org/licenses/by/4.0/),
and code samples are licensed under the
[Apache 2.0 License](https://www.apache.org/licenses/LICENSE-2.0).
