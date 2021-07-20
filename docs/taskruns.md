<!--
---
linkTitle: "TaskRuns"
weight: 300
---
-->

# `TaskRuns`

- [Overview](#taskruns)
- [Configuring a `TaskRun`](#configuring-a-taskrun)
  - [Specifying the target `Task`](#specifying-the-target-task)
  - [Tekton Bundles](#tekton-bundles)
  - [Specifying `Parameters`](#specifying-parameters)
  - [Specifying `Resources`](#specifying-resources)
  - [Specifying `ServiceAccount` credentials](#specifying-serviceaccount-credentials)
  - [Specifying a `Pod` template](#specifying-a-pod-template)
  - [Specifying `Workspaces`](#specifying-workspaces)
  - [Specifying `Sidecars`](#specifying-sidecars)
  - [Specifying `LimitRange` values](#specifying-limitrange-values)
  - [Configuring the failure timeout](#configuring-the-failure-timeout)
- [Monitoring execution status](#monitoring-execution-status)
  - [Monitoring `Steps`](#monitoring-steps)
  - [Monitoring `Results`](#monitoring-results)
- [Cancelling a `TaskRun`](#cancelling-a-taskrun)
- [Debugging a `TaskRun`](#debugging-a-taskrun)
- [Events](events.md#taskruns)
- [Running a TaskRun Hermetically](hermetic.md)
- [Code examples](#code-examples)
  - [Example `TaskRun` with a referenced `Task`](#example-taskrun-with-a-referenced-task)
  - [Example `TaskRun` with an embedded `Task`](#example-taskrun-with-an-embedded-task)
  - [Reusing a `Task`](#reusing-a-task)
  - [Using custom `ServiceAccount` credentials](#using-custom-serviceaccount-credentials)
  - [Running step containers as a non-root user](#running-step-containers-as-a-non-root-user)

# Overview

A `TaskRun` allows you to instantiate and execute a [`Task`](tasks.md) on-cluster. A `Task` specifies one or more
`Steps` that execute container images and each container image performs a specific piece of build work. A `TaskRun` executes the
`Steps` in the `Task` in the order they are specified until all `Steps` have executed successfully or a failure occurs.

## Configuring a `TaskRun`

A `TaskRun` definition supports the following fields:

- Required:
  - [`apiVersion`][kubernetes-overview] - Specifies the API version, for example
    `tekton.dev/v1beta1`.
  - [`kind`][kubernetes-overview] - Identifies this resource object as a `TaskRun` object.
  - [`metadata`][kubernetes-overview] - Specifies the metadata that uniquely identifies the
    `TaskRun`, such as a `name`.
  - [`spec`][kubernetes-overview] - Specifies the configuration for the `TaskRun`.
    - [`taskRef` or `taskSpec`](#specifying-the-target-task) - Specifies the `Tasks` that the
    `TaskRun` will execute.
- Optional:
  - [`serviceAccountName`](#specifying-serviceaccount-credentials) - Specifies a `ServiceAccount`
    object that provides custom credentials for executing the `TaskRun`.
  - [`params`](#specifying-parameters) - Specifies the desired execution parameters for the `Task`.
  - [`resources`](#specifying-resources) - Specifies the desired `PipelineResource` values.
    - [`inputs`](#specifying-resources) - Specifies the input resources.
    - [`outputs`](#specifying-resources) - Specifies the output resources.
  - [`timeout`](#configuring-the-failure-timeout) - Specifies the timeout before the `TaskRun` fails.
  - [`podTemplate`](#specifying-a-pod-template) - Specifies a [`Pod` template](podtemplates.md) to use as
    the starting point for configuring the `Pods` for the `Task`.
  - [`workspaces`](#specifying-workspaces) - Specifies the physical volumes to use for the
    [`Workspaces`](workspaces.md#using-workspaces-in-tasks) declared by a `Task`.

[kubernetes-overview]:
  https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/#required-fields

### Specifying the target `Task`

To specify the `Task` you want to execute in your `TaskRun`, use the `taskRef` field as shown below:

```yaml
spec:
  taskRef:
    name: read-task
```

You can also embed the desired `Task` definition directly in the `TaskRun` using the `taskSpec` field:

```yaml
spec:
  taskSpec:
    resources:
      inputs:
        - name: workspace
          type: git
    steps:
      - name: build-and-push
        image: gcr.io/kaniko-project/executor:v0.17.1
        # specifying DOCKER_CONFIG is required to allow kaniko to detect docker credential
        env:
          - name: "DOCKER_CONFIG"
            value: "/tekton/home/.docker/"
        command:
          - /kaniko/executor
        args:
          - --destination=gcr.io/my-project/gohelloworld
```

### Tekton Bundles

**Note: This is only allowed if `enable-tekton-oci-bundles` is set to
`"true"` or `enable-api-fields` is set to `"alpha"` in the `feature-flags`
configmap, see [`install.md`](./install.md#customizing-the-pipelines-controller-behavior)**

You may also reference `Tasks` that are defined outside of your cluster using `Tekton
Bundles`. A `Tekton Bundle` is an OCI artifact that contains Tekton resources like `Tasks`
which can be referenced within a `taskRef`.

```yaml
spec:
taskRef:
  name: echo-task
  bundle: docker.io/myrepo/mycatalog
```

Here, the `bundle` field is the full reference url to the artifact. The name is the
`metadata.name` field of the `Task`.

You may also specify a `tag` as you would with a Docker image which will give you a repeatable reference to a `Task`.

```yaml
spec:
taskRef:
  name: echo-task
  bundle: docker.io/myrepo/mycatalog:v1.0.1
```

You may also specify a fixed digest instead of a tag which ensures the referenced task is constant.

```yaml
spec:
taskRef:
  name: echo-task
  bundle: docker.io/myrepo/mycatalog@sha256:abc123
```

A working example can be found [here](../examples/v1beta1/taskruns/no-ci/tekton-bundles.yaml).

Any of the above options will fetch the image using the `ImagePullSecrets` attached to the
`ServiceAccount` specified in the `TaskRun`. See the [Service Account](#service-account)
section for details on how to configure a `ServiceAccount` on a `TaskRun`. The `TaskRun`
will then run that `Task` without registering it in the cluster allowing multiple versions
of the same named `Task` to be run at once.

`Tekton Bundles` may be constructed with any toolsets that produces valid OCI image artifacts so long as
the artifact adheres to the [contract](tekton-bundle-contracts.md). Additionally, you may also use the `tkn`
cli *(coming soon)*.

### Specifying `Parameters`

If a `Task` has [`parameters`](tasks.md#parameters), you can use the `params` field to specify their values:

```yaml
spec:
  params:
    - name: flags
      value: -someflag
```

**Note:** If a parameter does not have an implicit default value, you must explicitly set its value.

### Specifying `Resources`

If a `Task` requires [`Resources`](tasks.md#specifying-resources) (that is, `inputs` and `outputs`) you must
specify them in your `TaskRun` definition. You can specify `Resources` by reference to existing
[`PipelineResource` objects](resources.md) or embed their definitions directly in the `TaskRun`.

**Note:** A `TaskRun` can use *either* a referenced *or* an embedded `Resource` but not both simultaneously.

Below is an example of specifying `Resources` by reference:

```yaml
spec:
  resources:
    inputs:
      - name: workspace
        resourceRef:
          name: java-git-resource
    outputs:
      - name: image
        resourceRef:
          name: my-app-image
```

And here is an example of specifying `Resources` by embedding their definitions:

```yaml
spec:
  resources:
    inputs:
      - name: workspace
        resourceSpec:
          type: git
          params:
            - name: url
              value: https://github.com/pivotal-nader-ziada/gohelloworld
```

**Note:** You can use the `paths` field to [override the paths to a `Resource`](resources.md#overriding-where-resources-are-copied-from).

### Specifying `Resource` limits

Each Step in a Task can specify its resource requirements. See
[Defining `Steps`](tasks.md#defining-steps)

### Specifying a `Pod` template

You can specify a [`Pod` template](podtemplates.md) configuration that will serve as the configuration starting
point for the `Pod` in which the container images specified in your `Task` will execute. This allows you to
customize the `Pod` configuration specifically for that `TaskRun`.

In the following example, the `Task` specifies a `volumeMount` (`my-cache`) object, also provided by the `TaskRun`,
using a `PersistentVolumeClaim` volume. A specific scheduler is also configured in the  `SchedulerName` field.
The `Pod` executes with regular (non-root) user permissions.

```yaml
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: mytask
  namespace: default
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
kind: TaskRun
metadata:
  name: mytaskrun
  namespace: default
spec:
  taskRef:
    name: mytask
  podTemplate:
    schedulerName: volcano
    securityContext:
      runAsNonRoot: true
      runAsUser: 1001
    volumes:
      - name: my-cache
        persistentVolumeClaim:
          claimName: my-volume-claim
```

### Specifying `Workspaces`

If a `Task` specifies one or more `Workspaces`, you must map those `Workspaces` to
the corresponding physical volumes in your `TaskRun` definition. For example, you
can map a `PersistentVolumeClaim` volume to a `Workspace` as follows:

```yaml
workspaces:
  - name: myworkspace # must match workspace name in the Task
    persistentVolumeClaim:
      claimName: mypvc # this PVC must already exist
    subPath: my-subdir
```

For more information, see the following topics:
- For information mapping `Workspaces` to `Volumes`, see [Using `Workspace` variables in `TaskRuns`](workspaces.md#using-workspace-variables-in-taskruns).
- For a list of supported `Volume` types, see [Specifying `VolumeSources` in `Workspaces`](workspaces.md#specifying-volumesources-in-workspaces).
- For an end-to-end example, see [`Workspaces` in a `TaskRun`](../examples/v1beta1/taskruns/workspace.yaml).

### Specifying `Sidecars`

A `Sidecar` is a container that runs alongside the containers specified
in the `Steps` of a task to provide auxiliary support to the execution of
those `Steps`. For example, a `Sidecar` can run a logging daemon, a service
that updates files on a shared volume, or a network proxy.

Tekton supports the injection of `Sidecars` into a `Pod` belonging to
a `TaskRun` with the condition that each `Sidecar` running inside the
`Pod` are terminated as soon as all `Steps` in the `Task` complete execution.
This might result in the `Pod` including each affected `Sidecar` with a
retry count of 1 and a different container image than expected.

We are aware of the following issues affecting Tekton's implementation of `Sidecars`:

- The configured `nop` image **must not** provide the command that the
`Sidecar` is expected to run, otherwise it will not exit, resulting in the `Sidecar`
running forever and the Task eventually timing out. For more information, see the
[associated issue](https://github.com/tektoncd/pipeline/issues/1347).

- The `kubectl get pods` command returns the status of the `Pod` as "Completed" if a
`Sidecar` exits successfully and as "Error" if a `Sidecar` exits with an error,
disregarding the exit codes of the container images that actually executed the `Steps`
inside the `Pod`. Only the above command is affected. The `Pod's` description correctly
denotes a "Failed" status and the container statuses correctly denote their exit codes
and reasons.

### Specifying `LimitRange` values

In order to only consume the bare minimum amount of resources needed to execute one `Step` at a
time from the invoked `Task`, Tekton only requests the *maximum* values for CPU, memory, and ephemeral
storage from within each `Step`. This is sufficient as `Steps` only execute one at a time in the `Pod`.
Requests other than the maximum values are set to zero.

When a [`LimitRange`](https://kubernetes.io/docs/concepts/policy/limit-range/) parameter is present in
the namespace in which `TaskRuns` are executing and *minimum* values are specified for container resource requests,
Tekton searches through all `LimitRange` values present in the namespace and uses the *minimums* instead of 0.

For more information, see the [`LimitRange` code example](../examples/v1beta1/taskruns/no-ci/limitrange.yaml).

## Configuring the failure timeout

You can use the `timeout` field to set the `TaskRun's` desired timeout value. If you do not specify this
value for the `TaskRun`, the global default timeout value applies. If you set the timeout to 0, the `TaskRun` will
have no timeout and will run until it completes successfully or fails from an error.

The global default timeout is set to 60 minutes when you first install Tekton. You can set
a different global default timeout value using the `default-timeout-minutes` field in
[`config/config-defaults.yaml`](./../config/config-defaults.yaml). If you set the global timeout to 0,
all `TaskRuns` that do not have a timeout set will have no timeout and will run until it completes successfully
or fails from an error.

The `timeout` value is a `duration` conforming to Go's
[`ParseDuration`](https://golang.org/pkg/time/#ParseDuration) format. For example, valid
values are `1h30m`, `1h`, `1m`, `60s`, and `0`.

If a `TaskRun` runs longer than its timeout value, the pod associated with the `TaskRun` will be deleted. This
means that the logs of the `TaskRun` are not preserved. The deletion of the `TaskRun` pod is necessary in order to
stop `TaskRun` step containers from running.

### Specifying `ServiceAccount' credentials

You can execute the `Task` in your `TaskRun` with a specific set of credentials by
specifying a `ServiceAccount` object name in the `serviceAccountName` field in your `TaskRun`
definition. If you do not explicitly specify this, the `TaskRun` executes with the credentials
specified in the `configmap-defaults` `ConfigMap`. If this default is not specified, `TaskRuns`
will execute with the [`default` service account](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#use-the-default-service-account-to-access-the-api-server)
set for the target [`namespace`](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/).

For more information, see [`ServiceAccount`](auth.md).

## Monitoring execution status

As your `TaskRun` executes, its `status` field accumulates information on the execution of each `Step`
as well as the `TaskRun` as a whole. This information includes start and stop times, exit codes, the
fully-qualified name of the container image, and the corresponding digest.

**Note:** If any `Pods` have been [`OOMKilled`](https://kubernetes.io/docs/tasks/administer-cluster/out-of-resource/)
by Kubernetes, the `TaskRun` is marked as failed even if its exit code is 0.

The following example shows the `status` field of a `TaskRun` that has executed successfully:

```yaml
completionTime: "2019-08-12T18:22:57Z"
conditions:
  - lastTransitionTime: "2019-08-12T18:22:57Z"
    message: All Steps have completed executing
    reason: Succeeded
    status: "True"
    type: Succeeded
podName: status-taskrun-pod-6488ef
startTime: "2019-08-12T18:22:51Z"
steps:
  - container: step-hello
    imageID: docker-pullable://busybox@sha256:895ab622e92e18d6b461d671081757af7dbaa3b00e3e28e12505af7817f73649
    name: hello
    terminated:
      containerID: docker://d5a54f5bbb8e7a6fd3bc7761b78410403244cf4c9c5822087fb0209bf59e3621
      exitCode: 0
      finishedAt: "2019-08-12T18:22:56Z"
      reason: Completed
      startedAt: "2019-08-12T18:22:54Z"
  ```

The following tables shows how to read the overall status of a `TaskRun`:

`status`|`reason`|`completionTime` is set|Description
:-------|:-------|:---------------------:|--------------:
Unknown|Started|No|The TaskRun has just been picked up by the controller.
Unknown|Pending|No|The TaskRun is waiting on a Pod in status Pending.
Unknown|Running|No|The TaskRun has been validate and started to perform its work.
Unknown|TaskRunCancelled|No|The user requested the TaskRun to be cancelled. Cancellation has not be done yet.
True|Succeeded|Yes|The TaskRun completed successfully.
False|Failed|Yes|The TaskRun failed because one of the steps failed.
False|\[Error message\]|No|The TaskRun encountered a non-permanent error, and it's still running. It may ultimately succeed.
False|\[Error message\]|Yes|The TaskRun failed with a permanent error (usually validation).
False|TaskRunCancelled|Yes|The TaskRun was cancelled successfully.
False|TaskRunTimeout|Yes|The TaskRun timed out.

When a `TaskRun` changes status, [events](events.md#taskruns) are triggered accordingly.

### Monitoring `Steps`

If multiple `Steps` are defined in the `Task` invoked by the `TaskRun`, you can monitor their execution
status in the `status.steps` field using the following command, where `<name>` is the name of the target
`TaskRun`:

```bash
kubectl get taskrun <name> -o yaml
```

The exact Task Spec used to instantiate the TaskRun is also included in the Status for full auditability.

### Steps

The corresponding statuses appear in the `status.steps` list in the order in which the `Steps` have been
specified in the `Task` definition.

### Monitoring `Results`

If one or more `results` fields have been specified in the invoked `Task`, the `TaskRun's` execution
status will include a `Task Results` section, in which the `Results` appear verbatim, including original
line returns and whitespace. For example:

```yaml
Status:
  # […]
  Steps:
  # […]
  Task Results:
    Name:   current-date-human-readable
    Value:  Thu Jan 23 16:29:06 UTC 2020

    Name:   current-date-unix-timestamp
    Value:  1579796946

```

## Cancelling a `TaskRun`

To cancel a `TaskRun` that's currently executing, update its status to mark it as cancelled.

When you cancel a TaskRun, the running pod associated with that `TaskRun` is deleted. This
means that the logs of the `TaskRun` are not preserved. The deletion of the `TaskRun` pod is necessary
in order to stop `TaskRun` step containers from running.

Example of cancelling a `TaskRun`:

```yaml
apiVersion: tekton.dev/v1alpha1
kind: TaskRun
metadata:
  name: go-example-git
spec:
  # […]
  status: "TaskRunCancelled"
```


### Debugging a `TaskRun`

#### Breakpoint on Failure

TaskRuns can be halted on failure for troubleshooting by providing the following spec patch as seen below.

```yaml
spec:
  debug:
    breakpoint: ["onFailure"]
```

Upon failure of a step, the TaskRun Pod execution is halted. If ths TaskRun Pod continues to run without any lifecycle
change done by the user (running the debug-continue or debug-fail-continue script) the TaskRun would be subject to 
[TaskRunTimeout](#configuring-the-failure-timeout). 
During this time, the user/client can get remote shell access to the step container with a command such as the following.

```bash
kubectl exec -it print-date-d7tj5-pod-w5qrn -c step-print-date-human-readable 
```

#### Debug Environment

After the user/client has access to the container environment, they can scour for any missing parts because of which 
their step might have failed. 

To control the lifecycle of the step to mark it as a success or a failure or close the breakpoint, there are scripts
provided in the `/tekton/debug/scripts` directory in the container. The following are the scripts and the tasks they
perform :-

`debug-continue`: Mark the step as a success and exit the breakpoint.

`debug-fail-continue`: Mark the step as a failure and exit the breakpoint.

*More information on the inner workings of debug can be found in the [Debug documentation](debug.md)*

## Code examples

To better understand `TaskRuns`, study the following code examples:

- [Example `TaskRun` with a referenced `Task`](#example-taskrun-with-a-referenced-task)
- [Example `TaskRun` with an embedded `Task`](#example-taskrun-with-an-embedded-task)
- [Example of reusing a `Task`](#example-of-reusing-a-task)
- [Example of specifying a `ServiceAccount`](#example-of-specifying-a-service-account)

### Example `TaskRun` with a referenced `Task`

In this example, a `TaskRun` named `read-repo-run` invokes and executes an existing
`Task` named `read-task`. This `Task` uses a git input resource that the `TaskRun`
references as `go-example-git`.

```yaml
apiVersion: tekton.dev/v1alpha1
kind: PipelineResource
metadata:
  name: go-example-git
spec:
  type: git
  params:
    - name: url
      value: https://github.com/pivotal-nader-ziada/gohelloworld
---
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: read-task
spec:
  resources:
    inputs:
      - name: workspace
        type: git
  steps:
    - name: readme
      image: ubuntu
      script: cat workspace/README.md
---
apiVersion: tekton.dev/v1beta1
kind: TaskRun
metadata:
  name: read-repo-run
spec:
  taskRef:
    name: read-task
  resources:
    inputs:
      - name: workspace
        resourceRef:
          name: go-example-git
```

### Example `TaskRun` with an embedded `Task`

In this example, a `TaskRun` named `build-push-task-run-2` directly executes
a `Task` from its definition embedded in the `TaskRun's` `taskSpec` field:

```yaml
apiVersion: tekton.dev/v1alpha1
kind: PipelineResource
metadata:
  name: go-example-git
spec:
  type: git
  params:
    - name: url
      value: https://github.com/pivotal-nader-ziada/gohelloworld
---
apiVersion: tekton.dev/v1beta1
kind: TaskRun
metadata:
  name: build-push-task-run-2
spec:
  resources:
    inputs:
      - name: workspace
        resourceRef:
          name: go-example-git
  taskSpec:
    resources:
      inputs:
        - name: workspace
          type: git
    steps:
      - name: build-and-push
        image: gcr.io/kaniko-project/executor:v0.17.1
        # specifying DOCKER_CONFIG is required to allow kaniko to detect docker credential
        env:
          - name: "DOCKER_CONFIG"
            value: "/tekton/home/.docker/"
        command:
          - /kaniko/executor
        args:
          - --destination=gcr.io/my-project/gohelloworld
```

You can also embed resource definitions in your `TaskRun`. In the example below, a git resource
definition provides input for the `TaskRun` named `read-repo`:

```yaml
apiVersion: tekton.dev/v1beta1
kind: TaskRun
metadata:
  name: read-repo
spec:
  taskRef:
    name: read-task
  resources:
    inputs:
      - name: workspace
        resourceSpec:
          type: git
          params:
            - name: url
              value: https://github.com/pivotal-nader-ziada/gohelloworld
```

### Reusing a `Task`

The following example illustrates the reuse of the same `Task`. Below, you can see
several `TaskRuns` that instantiate a `Task` named `dockerfile-build-and-push`. The
`TaskRuns` reference different `Resources` as their inputs.
See [Building and pushing a Docker image](tasks.md#building-and-pushing-a-docker-image)
for the full definition of this example `Task.`

This `TaskRun` builds `mchmarny/rester-tester`:

```yaml
# This is the referenced PipelineResource
metadata:
  name: mchmarny-repo
spec:
  type: git
  params:
    - name: url
      value: https://github.com/mchmarny/rester-tester.git
```

```yaml
# This is the TaskRun
spec:
  taskRef:
    name: dockerfile-build-and-push
  params:
    - name: IMAGE
      value: gcr.io/my-project/rester-tester
  resources:
    inputs:
      - name: workspace
        resourceRef:
          name: mchmarny-repo
```

This `TaskRun` builds the `wget` builder from `googlecloudplatform/cloud-builder`:

```yaml
# This is the referenced PipelineResource
metadata:
  name: cloud-builder-repo
spec:
  type: git
  params:
    - name: url
      value: https://github.com/googlecloudplatform/cloud-builders.git
```

```yaml
# This is the TaskRun
spec:
  taskRef:
    name: dockerfile-build-and-push
  params:
    - name: IMAGE
      value: gcr.io/my-project/wget
    # Optional override to specify the subdirectory containing the Dockerfile
    - name: DIRECTORY
      value: /workspace/wget
  resources:
    inputs:
      - name: workspace
        resourceRef:
          name: cloud-builder-repo
```

This `TaskRun` builds the `docker` builder from `googlecloudplatform/cloud-builder` with `17.06.1`:

```yaml
# This is the referenced PipelineResource
metadata:
  name: cloud-builder-repo
spec:
  type: git
  params:
    - name: url
      value: https://github.com/googlecloudplatform/cloud-builders.git
```

```yaml
# This is the TaskRun
spec:
  taskRef:
    name: dockerfile-build-and-push
  params:
    - name: IMAGE
      value: gcr.io/my-project/docker
    # Optional overrides
    - name: DIRECTORY
      value: /workspace/docker
    - name: DOCKERFILE_NAME
      value: Dockerfile-17.06.1
  resources:
    inputs:
      - name: workspace
        resourceRef:
          name: cloud-builder-repo
```

### Using custom `ServiceAccount` credentials

The example below illustrates how to specify a `ServiceAccount` to access a private `git` repository:

```yaml
apiVersion: tekton.dev/v1beta1
kind: TaskRun
metadata:
  name: test-task-with-serviceaccount-git-ssh
spec:
  serviceAccountName: test-task-robot-git-ssh
  resources:
    inputs:
      - name: workspace
        resourceSpec:
          type: git
  taskSpec:
    steps:
      - name: config
        image: ubuntu
        command: ["/bin/bash"]
        args: ["-c", "cat README.md"]
```

In the above code snippet, `serviceAccountName: test-build-robot-git-ssh` references the following
`ServiceAccount`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: test-task-robot-git-ssh
secrets:
  - name: test-git-ssh
```

And `name: test-git-ssh` references the following `Secret`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: test-git-ssh
  annotations:
    tekton.dev/git-0: github.com
type: kubernetes.io/ssh-auth
data:
  # Generated by:
  # cat id_rsa | base64 -w 0
  ssh-privatekey: LS0tLS1CRUdJTiBSU0EgUFJJVk.....[example]
  # Generated by:
  # ssh-keyscan github.com | base64 -w 0
  known_hosts: Z2l0aHViLmNvbSBzc2g.....[example]
```

### Running Step Containers as a Non Root User

All steps that do not require to be run as a root user should make use of TaskRun features to
designate the container for a step runs as a user without root permissions. As a best practice,
running containers as non root should be built into the container image to avoid any possibility
of the container being run as root. However, as a further measure of enforcing this practice,
TaskRun pod templates can be used to specify how containers should be run within a TaskRun pod.

An example of using a TaskRun pod template is shown below to specify that containers running via this
TaskRun's pod should run as non root and run as user 1001 if the container itself does not specify what
user to run as:

```yaml
apiVersion: tekton.dev/v1beta1
kind: TaskRun
metadata:
  generateName: show-non-root-steps-run-
spec:
  taskRef:
    name: show-non-root-steps
  podTemplate:
    securityContext:
      runAsNonRoot: true
      runAsUser: 1001
```

If a Task step specifies that it is to run as a different user than what is specified in the pod template,
the step's `securityContext` will be applied instead of what is specified at the pod level. An example of
this is available as a [TaskRun example](../examples/v1beta1/taskruns/run-steps-as-non-root.yaml).

More information about Pod and Container Security Contexts can be found via the [Kubernetes website](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-the-security-context-for-a-pod).

---

Except as otherwise noted, the content of this page is licensed under the
[Creative Commons Attribution 4.0 License](https://creativecommons.org/licenses/by/4.0/),
and code samples are licensed under the
[Apache 2.0 License](https://www.apache.org/licenses/LICENSE-2.0).
