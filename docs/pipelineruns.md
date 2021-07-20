<!--
---
linkTitle: "PipelineRuns"
weight: 500
---
-->
# PipelineRuns

- [PipelineRuns](#pipelineruns)
  - [Overview](#overview)
  - [Configuring a `PipelineRun`](#configuring-a-pipelinerun)
    - [Specifying the target `Pipeline`](#specifying-the-target-pipeline)
      - [Tekton Bundles](#tekton-bundles)
  - [Specifying `Resources`](#specifying-resources)
    - [Specifying `Parameters`](#specifying-parameters)
    - [Specifying custom `ServiceAccount` credentials](#specifying-custom-serviceaccount-credentials)
    - [Mapping `ServiceAccount` credentials to `Tasks`](#mapping-serviceaccount-credentials-to-tasks)
    - [Specifying a `Pod` template](#specifying-a-pod-template)
    - [Specifying taskRunSpecs](#specifying-taskrunspecs)
    - [Specifying `Workspaces`](#specifying-workspaces)
    - [Specifying `LimitRange` values](#specifying-limitrange-values)
    - [Configuring a failure timeout](#configuring-a-failure-timeout)
  - [Monitoring execution status](#monitoring-execution-status)
  - [Cancelling a `PipelineRun`](#cancelling-a-pipelinerun)
  - [Pending `PipelineRuns`](#pending-pipelineruns)



## Overview

A `PipelineRun` allows you to instantiate and execute a [`Pipeline`](pipelines.md) on-cluster.
A `Pipeline` specifies one or more `Tasks` in the desired order of execution. A `PipelineRun`
executes the `Tasks` in the `Pipeline` in the order they are specified until all `Tasks` have
executed successfully or a failure occurs.

**Note:** A `PipelineRun` automatically creates corresponding `TaskRuns` for every
`Task` in your `Pipeline`.

The `Status` field tracks the current state of a `PipelineRun`, and can be used to monitor
progress.
This field contains the status of every `TaskRun`, as well as the full `PipelineSpec` used
to instantiate this `PipelineRun`, for full auditability.

## Configuring a `PipelineRun`

A `PipelineRun` definition supports the following fields:

- Required:
  - [`apiVersion`][kubernetes-overview] - Specifies the API version. For example
    `tekton.dev/v1beta1`.
  - [`kind`][kubernetes-overview] - Indicates that this resource object is a `PipelineRun` object.
  - [`metadata`][kubernetes-overview] - Specifies the metadata that uniquely identifies the
    `PipelineRun` object. For example, a `name`.
  - [`spec`][kubernetes-overview] - Specifies the configuration information for
    this `PipelineRun` object.
    - [`pipelineRef` or `pipelineSpec`](#specifying-the-target-pipeline) - Specifies the target [`Pipeline`](pipelines.md).
- Optional:
  - [`resources`](#specifying-resources) - Specifies the [`PipelineResources`](resources.md) to provision
    for executing the target `Pipeline`.
  - [`params`](#specifying-parameters) - Specifies the desired execution parameters for the `Pipeline`.
  - [`serviceAccountName`](#specifying-custom-serviceaccount-credentials) - Specifies a `ServiceAccount`
    object that supplies specific execution credentials for the `Pipeline`.
  - [`serviceAccountNames`](#mapping-serviceaccount-credentials-to-tasks) - Maps specific `serviceAccountName` values
    to `Tasks` in the `Pipeline`. This overrides the credentials set for the entire `Pipeline`.
  - [`taskRunSpec`](#specifying-taskrunspecs) - Specifies a list of `PipelineRunTaskSpec` which allows for setting `ServiceAccountName` and [`Pod` template](./podtemplates.md) for each task. This overrides the `Pod` template set for the entire `Pipeline`.
  - [`timeout`](#configuring-a-failure-timeout) - Specifies the timeout before the `PipelineRun` fails.
  - [`podTemplate`](#specifying-a-pod-template) - Specifies a [`Pod` template](./podtemplates.md) to use as the basis
    for the configuration of the `Pod` that executes each `Task`.

[kubernetes-overview]:
  https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/#required-fields

### Specifying the target `Pipeline`

You must specify the target `Pipeline` that you want the `PipelineRun` to execute, either by referencing
an existing `Pipeline` definition, or embedding a `Pipeline` definition directly in the `PipelineRun`.

To specify the target `Pipeline` by reference, use the `pipelineRef` field:

```yaml
spec:
  pipelineRef:
    name: mypipeline
```
To embed a `Pipeline` definition in the `PipelineRun`, use the `pipelineSpec` field:

```yaml
spec:
  pipelineSpec:
    tasks:
      - name: task1
        taskRef:
          name: mytask
```

The `Pipeline` in the [`pipelineSpec` example](../examples/v1beta1/pipelineruns/pipelinerun-with-pipelinespec.yaml)
example displays morning and evening greetings. Once you create and execute it, you can check the logs for its `Pods`:

```bash
kubectl logs $(kubectl get pods -o name | grep pipelinerun-echo-greetings-echo-good-morning)
Good Morning, Bob!

kubectl logs $(kubectl get pods -o name | grep pipelinerun-echo-greetings-echo-good-night)
Good Night, Bob!
```

You can also embed a `Task` definition the embedded `Pipeline` definition:

```yaml
spec:
  pipelineSpec:
    tasks:
      - name: task1
        taskSpec:
          steps: ...
```

In the [`taskSpec` in `pipelineSpec` example](../examples/v1beta1/pipelineruns/pipelinerun-with-pipelinespec-and-taskspec.yaml)
it's `Tasks` all the way down!

You can also specify labels and annotations with `taskSpec` which are propagated to each `taskRun` and then to the
respective pods. These labels can be used to identify and filter pods for further actions (such as collecting pod metrics,
and cleaning up completed pod with certain labels, etc) even being part of one single Pipeline.

```yaml
spec:
  pipelineSpec:
    tasks:
      - name: task1
        taskSpec:
          metadata:
            labels:
              pipeline-sdk-type: kfp
        # ...
      - name: task2
        taskSpec:
          metadata:
            labels:
              pipeline-sdk-type: tfx
        # ...
```

#### Tekton Bundles

**Note: This is only allowed if `enable-tekton-oci-bundles` is set to
`"true"` in the `feature-flags` configmap, see [`install.md`](./install.md#customizing-the-pipelines-controller-behavior)**

You may also use a `Tekton Bundle` to reference a pipeline defined remotely.

 ```yaml
 spec:
   pipelineRef:
     name: mypipeline
     bundle: docker.io/myrepo/mycatalog:v1.0
 ```

The syntax and caveats are similar to using `Tekton Bundles` for  `Task` references
in [Pipelines](pipelines.md#tekton-bundles) or [TaskRuns](taskruns.md#tekton-bundles).

`Tekton Bundles` may be constructed with any toolsets that produce valid OCI image artifacts
so long as the artifact adheres to the [contract](tekton-bundle-contracts.md).


## Specifying `Resources`

A `Pipeline` requires [`PipelineResources`](resources.md) to provide inputs and store outputs
for the `Tasks` that comprise it. You must provision those resources in the `resources` field
in the `spec` section of the `PipelineRun` definition.

A `Pipeline` may require you to provision a number of different resources. For example:

- When executing a `Pipeline` against a pull request, the triggering
  system must specify the commit-ish of a `git` resource.
- When executing a `Pipeline` manually against your own environment, you
  must provision your GitHub fork using the `git` resource; your image
  registry using the `image` resource; and your Kubernetes cluster using the
  `cluster` resource.

You can reference a `PipelineResources` using the `resourceRef` field:

```yaml
spec:
  resources:
    - name: source-repo
      resourceRef:
        name: skaffold-git
    - name: web-image
      resourceRef:
        name: skaffold-image-leeroy-web
    - name: app-image
      resourceRef:
        name: skaffold-image-leeroy-app
```

You can also embed a `PipelineResource` definition in the `PipelineRun` using the `resourceSpec` field:

```yaml
spec:
  resources:
    - name: source-repo
      resourceSpec:
        type: git
        params:
          - name: revision
            value: v0.32.0
          - name: url
            value: https://github.com/GoogleContainerTools/skaffold
    - name: web-image
      resourceSpec:
        type: image
        params:
          - name: url
            value: gcr.io/christiewilson-catfactory/leeroy-web
    - name: app-image
      resourceSpec:
        type: image
        params:
          - name: url
            value: gcr.io/christiewilson-catfactory/leeroy-app
```

**Note:** All `persistentVolumeClaims` specified within a `PipelineRun` are bound
until their respective `Pods` or the entire `PipelineRun` are deleted. This also applies
to all `persistentVolumeClaims` generated internally.

### Specifying `Parameters`

You can specify `Parameters` that you want to pass to the `Pipeline` during execution,
including different values of the same parameter for different `Tasks` in the `Pipeline`.

**Note:** You must specify all the `Parameters` that the `Pipeline` expects. Parameters
that have default values specified in Pipeline are not required to be provided by PipelineRun.

For example:

```yaml
spec:
  params:
    - name: pl-param-x
      value: "100"
    - name: pl-param-y
      value: "500"
```
You can pass in extra `Parameters` if needed depending on your use cases. An example use
case is when your CI system autogenerates `PipelineRuns` and it has `Parameters` it wants to
provide to all `PipelineRuns`. Because you can pass in extra `Parameters`, you don't have to
go through the complexity of checking each `Pipeline` and providing only the required params.

### Specifying custom `ServiceAccount` credentials

You can execute the `Pipeline` in your `PipelineRun` with a specific set of credentials by
specifying a `ServiceAccount` object name in the `serviceAccountName` field in your `PipelineRun`
definition. If you do not explicitly specify this, the `TaskRuns` created by your `PipelineRun`
will execute with the credentials specified in the `configmap-defaults` `ConfigMap`. If this
default is not specified, the `TaskRuns` will execute with the [`default` service account](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#use-the-default-service-account-to-access-the-api-server)
set for the target [`namespace`](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/).

For more information, see [`ServiceAccount`](auth.md).

[`Custom tasks`](pipelines.md#using-custom-tasks) may or may not use a service account name.
Consult the documentation of the custom task that you are using to determine whether it supports a service account name.

### Mapping `ServiceAccount` credentials to `Tasks`

If you require more granularity in specifying execution credentials, use the `serviceAccountNames` field to
map a specific `serviceAccountName` value to a specific `Task` in the `Pipeline`. This overrides the global
`serviceAccountName` you may have set for the `Pipeline` as described in the previous section.

For example, if you specify these mappings:

```yaml
spec:
  serviceAccountName: sa-1
  serviceAccountNames:
    - taskName: build-task
      serviceAccountName: sa-for-build
```

for this `Pipeline`:

```yaml
kind: Pipeline
spec:
  tasks:
    - name: build-task
      taskRef:
        name: build-push
    - name: test-task
      taskRef:
        name: test
```

then `test-task` will execute using the `sa-1` account while `build-task` will execute with `sa-for-build`.

### Specifying a `Pod` template

You can specify a [`Pod` template](podtemplates.md) configuration that will serve as the configuration starting
point for the `Pod` in which the container images specified in your `Tasks` will execute. This allows you to
customize the `Pod` configuration specifically for each `TaskRun`.

In the following example, the `Task` defines a `volumeMount` object named `my-cache`. The `PipelineRun`
provisions this object for the `Task` using a `persistentVolumeClaim` and executes it as user 1001.

```yaml
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: mytask
spec:
  steps:
    - name: writesomething
      image: ubuntu
      command: ["bash", "-c"]
      args: ["echo 'foo' > /my-cache/bar"]
      volumeMounts:
        - name: my-cache
          mountPath: /my-cache
---
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: mypipeline
spec:
  tasks:
    - name: task1
      taskRef:
        name: mytask
---
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  name: mypipelinerun
spec:
  pipelineRef:
    name: mypipeline
  podTemplate:
    securityContext:
      runAsNonRoot: true
      runAsUser: 1001
    volumes:
      - name: my-cache
        persistentVolumeClaim:
          claimName: my-volume-claim
```

[`Custom tasks`](pipelines.md#using-custom-tasks) may or may not use a pod template.
Consult the documentation of the custom task that you are using to determine whether it supports a pod template.

### Specifying taskRunSpecs

Specifies a list of `PipelineTaskRunSpec` which contains `TaskServiceAccountName`, `TaskPodTemplate`
and `PipelineTaskName`. Mapping the specs to the corresponding `Task` based upon the `TaskName` a PipelineTask
will run with the configured  `TaskServiceAccountName` and `TaskPodTemplate` overwriting the pipeline
wide `ServiceAccountName`  and [`podTemplate`](./podtemplates.md) configuration,
for example:

```yaml
spec:
  podTemplate:
    securityContext:
      runAsUser: 1000
      runAsGroup: 2000
      fsGroup: 3000
  taskRunSpecs:
    - pipelineTaskName: build-task
      taskServiceAccountName: sa-for-build
      taskPodTemplate:
        nodeSelector:
          disktype: ssd
```

If used with this `Pipeline`,  `build-task` will use the task specific `PodTemplate` (where `nodeSelector` has `disktype` equal to `ssd`).

### Specifying `Workspaces`

If your `Pipeline` specifies one or more `Workspaces`, you must map those `Workspaces` to
the corresponding physical volumes in your `PipelineRun` definition. For example, you
can map a `PersistentVolumeClaim` volume to a `Workspace` as follows:

```yaml
workspaces:
  - name: myworkspace # must match workspace name in Task
    persistentVolumeClaim:
      claimName: mypvc # this PVC must already exist
    subPath: my-subdir
```

For more information, see the following topics:
- For information on mapping `Workspaces` to `Volumes`, see [Specifying `Workspaces` in `PipelineRuns`](workspaces.md#specifying-workspaces-in-pipelineruns).
- For a list of supported `Volume` types, see [Specifying `VolumeSources` in `Workspaces`](workspaces.md#specifying-volumesources-in-workspaces).
- For an end-to-end example, see [`Workspaces` in a `PipelineRun`](../examples/v1beta1/pipelineruns/workspaces.yaml).

[`Custom tasks`](pipelines.md#using-custom-tasks) may or may not use workspaces.
Consult the documentation of the custom task that you are using to determine whether it supports workspaces.

### Specifying `LimitRange` values

In order to only consume the bare minimum amount of resources needed to execute one `Step` at a
time from the invoked `Task`, Tekton only requests the *maximum* values for CPU, memory, and ephemeral
storage from within each `Step`. This is sufficient as `Steps` only execute one at a time in the `Pod`.
Requests other than the maximum values are set to zero.

When a [`LimitRange`](https://kubernetes.io/docs/concepts/policy/limit-range/) parameter is present in
the namespace in which `PipelineRuns` are executing and *minimum* values are specified for container resource requests,
Tekton searches through all `LimitRange` values present in the namespace and uses the *minimums* instead of 0.

For more information, see the [`LimitRange` code example](../examples/v1beta1/pipelineruns/no-ci/limitrange.yaml).

### Configuring a failure timeout

You can use the `timeout` field to set the `PipelineRun's` desired timeout value in minutes.
If you do not specify this value in the `PipelineRun`, the global default timeout value applies.
If you set the timeout to 0, the `PipelineRun` fails immediately upon encountering an error.

> :warning: ** `timeout`will be deprecated in future versions. Consider using `timeouts` instead.

You can use the `timeouts` field to set the `PipelineRun's` desired timeout value in minutes.  There are three sub-fields than can be used to specify failures timeout for the entire pipeline, for tasks, and for finally tasks.  

```yaml
timeouts:
  pipeline: "0h0m60s"
  tasks: "0h0m40s"
  finally: "0h0m20s"
```
All three sub-fields are optional, and will be automatically processed according to the following constraint:
* `timeouts.pipeline >= timeouts.tasks + timeouts.finally`

You may combine the timeouts as follow:

Combination 1: Set the timeout for the entire `pipeline` and reserve a portion of it for `tasks`.

```yaml
kind: PipelineRun
spec:
  timeouts:
    pipeline: "0h4m0s"
    tasks: "0h1m0s"
```

Combination 2: Set the timeout for the entire `pipeline` and reserve a portion of it for `finally`.

```yaml
kind: PipelineRun
spec:
  timeouts:
    pipeline: "0h4m0s"
    finally: "0h3m0s"
```

If you do not specify the `timeout` value or `timeouts.pipeline` in the `PipelineRun`, the global default timeout value applies.
If you set the `timeout` value or `timeouts.pipeline` to 0, the `PipelineRun` fails immediately upon encountering an error.

The global default timeout is set to 60 minutes when you first install Tekton. You can set
a different global default timeout value using the `default-timeout-minutes` field in
[`config/config-defaults.yaml`](./../config/config-defaults.yaml).

The `timeout` value is a `duration` conforming to Go's
[`ParseDuration`](https://golang.org/pkg/time/#ParseDuration) format. For example, valid
values are `1h30m`, `1h`, `1m`, and `60s`. If you set the global timeout to 0, all `PipelineRuns`
that do not have an individual timeout set will fail immediately upon encountering an error.

## Monitoring execution status

As your `PipelineRun` executes, its `status` field accumulates information on the execution of each `TaskRun`
as well as the `PipelineRun` as a whole. This information includes the name of the pipeline `Task` associated
to a `TaskRun`, the complete [status of the `TaskRun`](taskruns.md#monitoring-execution-status) and details
about `Conditions` that may be associated to a `TaskRun`.

The following example shows an extract from the `status` field of a `PipelineRun` that has executed successfully:

```yaml
completionTime: "2020-05-04T02:19:14Z"
conditions:
  - lastTransitionTime: "2020-05-04T02:19:14Z"
    message: "Tasks Completed: 4, Skipped: 0"
    reason: Succeeded
    status: "True"
    type: Succeeded
startTime: "2020-05-04T02:00:11Z"
taskRuns:
  triggers-release-nightly-frwmw-build-ng2qk:
    pipelineTaskName: build
    status:
      completionTime: "2020-05-04T02:10:49Z"
      conditions:
        - lastTransitionTime: "2020-05-04T02:10:49Z"
          message: All Steps have completed executing
          reason: Succeeded
          status: "True"
          type: Succeeded
      podName: triggers-release-nightly-frwmw-build-ng2qk-pod-8vj99
      resourcesResult:
        - key: commit
          resourceRef:
            name: git-source-triggers-frwmw
          value: 9ab5a1234166a89db352afa28f499d596ebb48db
      startTime: "2020-05-04T02:05:07Z"
      steps:
        - container: step-build
          imageID: docker-pullable://golang@sha256:a90f2671330831830e229c3554ce118009681ef88af659cd98bfafd13d5594f9
          name: build
          terminated:
            containerID: docker://6b6471f501f59dbb7849f5cdde200f4eeb64302b862a27af68821a7fb2c25860
            exitCode: 0
            finishedAt: "2020-05-04T02:10:45Z"
            reason: Completed
            startedAt: "2020-05-04T02:06:24Z"
  ```

The following tables shows how to read the overall status of a `PipelineRun`.
Completion time is set once a `PipelineRun` reaches status `True` or `False`:

`status`|`reason`|`completionTime` is set|Description
:-------|:-------|:---------------------:|--------------:
Unknown|Started|No|The `PipelineRun` has just been picked up by the controller.
Unknown|Running|No|The `PipelineRun` has been validate and started to perform its work.
Unknown|PipelineRunCancelled|No|The user requested the PipelineRun to be cancelled. Cancellation has not be done yet.
True|Succeeded|Yes|The `PipelineRun` completed successfully.
True|Completed|Yes|The `PipelineRun` completed successfully, one or more Tasks were skipped.
False|Failed|Yes|The `PipelineRun` failed because one of the `TaskRuns` failed.
False|\[Error message\]|Yes|The `PipelineRun` failed with a permanent error (usually validation).
False|PipelineRunCancelled|Yes|The `PipelineRun` was cancelled successfully.
False|PipelineRunTimeout|Yes|The `PipelineRun` timed out.

When a `PipelineRun` changes status, [events](events.md#pipelineruns) are triggered accordingly.

When a `PipelineRun` has `Tasks` with [`when` expressions](pipelines.md#guard-task-execution-using-when-expressions):
- If the `when` expressions evaluate to `true`, the `Task` is executed then the `TaskRun` and its resolved `when` expressions will be listed in the `Task Runs` section of the `status` of the `PipelineRun`.
- If the `when` expressions evaluate to `false`, the `Task` is skipped then its name and its resolved `when` expressions will be listed in the `Skipped Tasks` section of the `status` of the `PipelineRun`.

```yaml
Conditions:
  Last Transition Time:  2020-08-27T15:07:34Z
  Message:               Tasks Completed: 1 (Failed: 0, Cancelled 0), Skipped: 1
  Reason:                Completed
  Status:                True
  Type:                  Succeeded
Skipped Tasks:
  Name:       skip-this-task
  When Expressions:
    Input:     foo
    Operator:  in
    Values:
      bar
    Input:     foo
    Operator:  notin
    Values:
      foo
Task Runs:
  pipelinerun-to-skip-task-run-this-task-r2djj:
    Pipeline Task Name:  run-this-task
    Status:
      ...
    When Expressions:
      Input:     foo
      Operator:  in
      Values:
        foo
```

## Cancelling a `PipelineRun`

To cancel a `PipelineRun` that's currently executing, update its definition
to mark it as "PipelineRunCancelled". When you do so, the spawned `TaskRuns` are also marked
as cancelled and all associated `Pods` are deleted. Pending final tasks are not scheduled. 
For example:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  name: go-example-git
spec:
  # […]
  status: "PipelineRunCancelled"
```

Warning: "PipelineRunCancelled" status is deprecated and would be removed in V1, please use "Cancelled" instead.  

## Gracefully cancelling a `PipelineRun`

[Graceful pipeline run termination](https://github.com/tektoncd/community/blob/main/teps/0058-graceful-pipeline-run-termination.md)
is currently an **_alpha feature_**.

To gracefully cancel a `PipelineRun` that's currently executing, update its definition
to mark it as "CancelledRunFinally". When you do so, the spawned `TaskRuns` are also marked
as cancelled and all associated `Pods` are deleted. Final tasks are scheduled normally. 
For example:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  name: go-example-git
spec:
  # […]
  status: "CancelledRunFinally"
```


## Gracefully stopping a `PipelineRun`

To gracefully stop a `PipelineRun` that's currently executing, update its definition
to mark it as "StoppedRunFinally". When you do so, the spawned `TaskRuns` are completed normally,
but no new non-final task is scheduled. Final tasks are executed afterwards.
For example:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  name: go-example-git
spec:
  # […]
  status: "StoppedRunFinally"
```

## Pending `PipelineRuns`

A `PipelineRun` can be created as a "pending" `PipelineRun` meaning that it will not actually be started until the pending status is cleared.

Note that a `PipelineRun` can only be marked "pending" before it has started, this setting is invalid after the `PipelineRun` has been started.

To mark a `PipelineRun` as pending, set `.spec.status` to `PipelineRunPending` when creating it:

```yaml
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
  name: go-example-git
spec:
  # […]
  status: "PipelineRunPending"
```

To start the PipelineRun, clear the `.spec.status` field. Alternatively, update the value to `PipelineRunCancelled` to cancel it.

---

Except as otherwise noted, the content of this page is licensed under the
[Creative Commons Attribution 4.0 License](https://creativecommons.org/licenses/by/4.0/),
and code samples are licensed under the
[Apache 2.0 License](https://www.apache.org/licenses/LICENSE-2.0).
