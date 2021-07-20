# Developer docs

This document is aimed at helping maintainers/developers of project understand
the complexity.

## How are resources shared between tasks

`PipelineRun` uses PVC to share `PipelineResources` between tasks. PVC volume is
mounted on path `/pvc` by PipelineRun.

- If a resource in a task is declared as output then the `TaskRun` controller
  adds a step to copy each output resource to the directory path
  `/pvc/task_name/resource_name`.

- If an input resource includes `from` condition then the `TaskRun` controller
  adds a step to copy from PVC directory path:
  `/pvc/previous_task/resource_name`.

If neither of these conditions are met, the PVC will not be created nor will
the GCS storage / S3 buckets be used.

Another alternative is to use a GCS storage or S3 bucket to share the artifacts.
This can be configured using a ConfigMap with the name `config-artifact-bucket`.

See the [installation docs](../install.md#how-are-resources-shared-between-tasks) for configuration details.

Both options provide the same functionality to the pipeline. The choice is based
on the infrastructure used, for example in some Kubernetes platforms, the
creation of a persistent volume could be slower than uploading/downloading files
to a bucket, or if the the cluster is running in multiple zones, the access to
the persistent volume can fail.

## How are inputs handled

Input resources, like source code (git) or artifacts, are dumped at path
`/workspace/task_resource_name`. 

- If input resource is declared as below, then resource will be copied to
 `/workspace/task_resource_name` directory `from` depended task PVC directory
 `/pvc/previous_task/resource_name`.
 
  ```yaml
  kind: Task
  metadata:
    name: get-gcs-task
    namespace: default
  spec:
    resources:
      inputs:
        - name: gcs-workspace
          type: storage
  ```

- Resource definition in task can have custom target directory. 
If `targetPath` is mentioned in task input resource as below then resource will be 
copied to `/workspace/outputstuff` directory `from` depended task PVC directory
 `/pvc/previous_task/resource_name`.

  ```yaml
  kind: Task
  metadata:
    name: get-gcs-task
    namespace: default
  spec:
    resources:
      inputs:
        - name: gcs-workspace
          type: storage
          targetPath: /workspace/outputstuff
  ```

## How are outputs handled

Output resources, like source code (git) or artifacts (storage resource), are
expected in directory path `/workspace/output/resource_name`.

- If resource has an output "action" like upload to blob storage, then the
  container step is added for this action.
- If there is PVC volume present (TaskRun holds owner reference to PipelineRun)
  then copy step is added as well.

- If the output resource is declared then the
  copy step includes resource being copied to PVC to path
  `/pvc/task_name/resource_name` from `/workspace/output/resource_name` like the
  following example.

  ```yaml
  kind: Task
  metadata:
    name: get-gcs-task
    namespace: default
  spec:
    resources:
      outputs:
        - name: gcs-workspace
          type: storage
  ```

- Same as input, if the output resource is declared with `TargetPath` then the copy step 
includes resource being copied to PVC to path `/pvc/task_name/resource_name` 
from `/workspace/outputstuff` like the following example.

  ```yaml
  kind: Task
  metadata:
    name: get-gcs-task
    namespace: default
  spec:
    resources:
      outputs:
        - name: gcs-workspace
          type: storage
          targetPath: /workspace/outputstuff
  ```

## Entrypoint rewriting and step ordering

`Entrypoint` is injected into the `Task` Container(s), wraps the `Task` step to
manage the execution order of the containers. The `entrypoint` binary has the
following arguments:

- `wait_file` - If specified, file to wait for
- `wait_file_content` - If specified, wait until the file has non-zero size
- `post_file` - If specified, file to write upon completion
- `entrypoint` - The command to run in the image being wrapped

As part of the PodSpec created by `TaskRun` the entrypoint for each `Task` step
is changed to the entrypoint binary with the mentioned arguments and a volume
with the binary and file(s) is mounted.

If the image is a private registry, the service account should include an
[ImagePullSecret](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account)

## Reserved directories

The `/tekton/` directory is reserved on containers for internal usage. Examples
of how this directory is used:

* `/workspace` - This directory is where [resources](#resources) and [workspaces](#workspaces)
  are mounted.
* `/tekton` - This directory is used for Tekton specific functionality:
  * These folders are [part of the Tekton API](../api_compatibility_policy.md):
    * `/tekton/results` is where [results](#results) are written to
      (path available to `Task` authors via [`$(results.name.path)`](../variables.md))
    * `/tekton/steps` is where the `step` exitCodes are written to
      (path available to `Task` authors via [`$(steps.<stepName>.exitCode.path)`](../variables.md#variables-available-in-a-task))
  * These folders are implementation details of Tekton and **users should not
    rely on this specific behavior as it may change in the future**:
    * `/tekton/tools` contains tools like the [entrypoint binary](#entrypoint-rewriting-and-step-ordering)
    * `/tekton/termination` is where the eventual [termination log message](https://kubernetes.io/docs/tasks/debug-application-cluster/determine-reason-pod-failure/#writing-and-reading-a-termination-message) is written to
    * [Sequencing step containers](#entrypoint-rewriting-and-step-ordering)
      is done using both `/tekton/downward/ready` and numbered files in `/tekton/tools`

## Handling of injected sidecars

Tekton has to take some special steps to support sidecars that are injected into
TaskRun Pods. Without intervention sidecars will typically run for the entire
lifetime of a Pod but in Tekton's case it's desirable for the sidecars to run
only as long as Steps take to complete. There's also a need for Tekton to
schedule the sidecars to start before a Task's Steps begin, just in case the
Steps rely on a sidecars behavior, for example to join an Istio service mesh.
To handle all of this, Tekton Pipelines implements the following lifecycle
for sidecar containers:

First, the [Downward API](https://kubernetes.io/docs/tasks/inject-data-application/downward-api-volume-expose-pod-information/#the-downward-api)
is used to project an annotation on the TaskRun's Pod into the `entrypoint`
container as a file. The annotation starts as an empty string, so the file
projected by the downward API has zero length. The entrypointer spins, waiting
for that file to have non-zero size.

The sidecar containers start up. Once they're all in a ready state, the
annotation is populated with string "READY", which in turn populates the
Downward API projected file. The entrypoint binary recognizes
that the projected file has a non-zero size and allows the Task's steps to
begin.

On completion of all steps in a Task the TaskRun reconciler stops any
sidecar containers. The `Image` field of any sidecar containers is swapped
to the nop image. Kubernetes observes the change and relaunches the container
with updated container image. The nop container image exits immediately
*because it does not provide the command that the sidecar is configured to run*.
The container is considered `Terminated` by Kubernetes and the TaskRun's Pod
stops.

There are known issues with the existing implementation of sidecars:

- When the `nop` image does provide the sidecar's command, the sidecar will continue to
run even after `nop` has been swapped into the sidecar container's image
field. See [the issue tracking this bug](https://github.com/tektoncd/pipeline/issues/1347)
for the issue tracking this bug. Until this issue is resolved the best way to avoid it is to
avoid overriding the `nop` image when deploying the tekton controller, or
ensuring that the overridden `nop` image contains as few commands as possible.

- `kubectl get pods` will show a Completed pod when a sidecar exits successfully
but an Error when the sidecar exits with an error. This is only apparent when
using `kubectl` to get the pods of a TaskRun, not when describing the Pod
using `kubectl describe pod ...` nor when looking at the TaskRun, but can be quite
confusing.

## How task results are defined and outputted by a task

Tasks can define results by adding a result on the task spec.
This is an example:

```yaml
apiVersion: tekton.dev/v1alpha1
kind: Task
metadata:
  name: print-date
  annotations:
    description: |
      A simple task that prints the date to make sure your cluster / Tekton is working properly.
spec:
  results:
    - name: "current-date"
      description: "The current date"
  steps:
    - name: print-date
      image: bash:latest
      args:
        - "-c"
        - |
          date > /tekton/results/current-date
```

The result is added to a file name with the specified result's name into the `/tekton/results` folder. This is then added to the
task run status.
Internally the results are a new argument `-results`to the entrypoint defined for the task. A user can defined more than one result for a
single task.

For this task definition,

```yaml
apiVersion: tekton.dev/v1alpha1
kind: Task
metadata:
  name: print-date
  annotations:
    description: |
      A simple task that prints the date to make sure your cluster / Tekton is working properly.
spec:
  results:
    - name: current-date-unix-timestamp
      description: The current date in unix timestamp format
    - name: current-date-human-readable
      description: The current date in humand readable format
  steps:
    - name: print-date-unix-timestamp
      image: bash:latest
      script: |
        #!/usr/bin/env bash
        date +%s | tee /tekton/results/current-date-unix-timestamp
    - name: print-date-human-readable
      image: bash:latest
      script: |
        #!/usr/bin/env bash
        date | tee /tekton/results/current-date-human-readable
```

you end up with this task run status:

```yaml
apiVersion: tekton.dev/v1alpha1
kind: TaskRun
# ...
status:
# ...
  taskResults:
    - name: current-date-human-readable
      value: |
        Wed Jan 22 19:47:26 UTC 2020
    - name: current-date-unix-timestamp
      value: |
        1579722445
```

Instead of hardcoding the path to the result file, the user can also use a variable. So `/tekton/results/current-date-unix-timestamp` can be replaced with: `$(results.current-date-unix-timestamp.path)`. This is more flexible if the path to result files ever changes.

### Known issues

- Task Results are returned to the TaskRun controller via the container's
termination message. At time of writing this has a capped maximum size of ["4096 bytes or 80 lines, whichever is smaller"](https://kubernetes.io/docs/tasks/debug-application-cluster/determine-reason-pod-failure/#customizing-the-termination-message).
This maximum size should not be considered the limit of a result's size. Tekton uses
the termination message to return other data to the controller as well. The general
advice should be that results are for very small pieces of data. The exact size
is going to be a product of the platform's settings and the amount of other
data Tekton needs to return for TaskRun book-keeping.

## How task results can be used in pipeline's tasks

Now that we have tasks that can return a result, the user can refer to a task result in a pipeline by using the syntax
`$(tasks.<task name>.results.<result name>)`. This will substitute the task result at the location of the variable.

```yaml
apiVersion: tekton.dev/v1alpha1
kind: Pipeline
metadata:
  name: sum-and-multiply-pipeline
  #...
  tasks:
    - name: sum-inputs
    #...
    - name: multiply-inputs
    #...
    - name: sum-and-multiply
      taskRef:
        name: sum
      params:
        - name: a
          value: "$(tasks.multiply-inputs.results.product)$(tasks.sum-inputs.results.sum)"
        - name: b
          value: "$(tasks.multiply-inputs.results.product)$(tasks.sum-inputs.results.sum)"
```

This results in:

```shell
tkn pipeline start sum-and-multiply-pipeline
? Value for param `a` of type `string`? (Default is `1`) 10
? Value for param `b` of type `string`? (Default is `1`) 15
Pipelinerun started: sum-and-multiply-pipeline-run-rgd9j

In order to track the pipelinerun progress run:
tkn pipelinerun logs sum-and-multiply-pipeline-run-rgd9j -f -n default
```

```shell
tkn pipelinerun logs sum-and-multiply-pipeline-run-rgd9j -f -n default
[multiply-inputs : product] 150

[sum-inputs : sum] 25

[sum-and-multiply : sum] 30050
```

As you can see, you can define multiple tasks in the same pipeline and use the result of more than one task inside another task parameter. The substitution is only done inside `pipeline.spec.tasks[].params[]`. For a complete example demonstrating Task Results in a Pipeline, see the [pipelinerun example](../../examples/v1beta1/pipelineruns/task_results_example.yaml).

## Support for running in multi-tenant configuration

In order to support potential multi-tenant configurations the roles of the controller are split into two:

    `tekton-pipelines-controller-cluster-access`: those permissions needed cluster-wide by the controller.
    `tekton-pipelines-controller-tenant-access`: those permissions needed on a namespace-by-namespace basis.

By default the roles are cluster-scoped for backwards-compatibility and ease-of-use. If you want to
start running a multi-tenant service you are able to bind `tekton-pipelines-controller-tenant-access`
using a `RoleBinding` instead of a `ClusterRoleBinding`, thereby limiting the access that the controller has to
specific tenant namespaces.

## Adding feature gated API fields

We've introduced a feature-flag called `enable-api-fields` to the
[config-feature-flags.yaml file](../../config/config-feature-flags.yaml) deployed
as part of our releases.

This field can be configured either to be `alpha` or `stable`. This
field is documented as part of our [install docs](../install.md#customizing-the-pipelines-controller-behavior).

For developers adding new features to Pipelines' CRDs we've got
a couple of helpful tools to make gating those features simpler
and to provide a consistent testing experience.

### Guarding Features with Feature Gates

Writing new features is made trickier when you need to support both
the existing stable behaviour as well as your new alpha behaviour.

In reconciler code you can guard your new features with an `if` statement
such as the following:

```go
alphaAPIEnabled := config.FromContextOrDefaults(ctx).FeatureFlags.EnableAPIFields == "alpha"
if alphaAPIEnabled {
  // new feature code goes here
} else {
  // existing stable code goes here
}
```

Notice that you'll need a context object to be passed into your function
for this to work. When writing new features keep in mind that you might
need to include this in your new function signatures.

### Guarding Validations with Feature Gates

Just because your application code might be correctly observing
the feature gate flag doesn't mean you're done yet! When a user submits
a Tekton resource it's validated by Pipelines' webhook. Here too you'll need
to ensure your new features aren't accidentally accepted when the feature gate
suggests they shouldn't be. We've got a helper function,
[`ValidateEnabledAPIFields`](../../pkg/apis/pipeline/v1beta1/version_validation.go),
to make validating the current feature gate easier. Use it like this:

```go
requiredVersion := config.AlphaAPIFields
// errs is an instance of *apis.FieldError, a common type in our validation code
errs = errs.Also(ValidateEnabledAPIFields(ctx, "your feature name", requiredVersion))
```

If the user's cluster isn't configured with the required feature gate it'll
return an error like this:

```
<your feature> requires "enable-api-fields" feature gate to be "alpha" but it is "stable"
```

### Unit Testing with Feature Gates

Any new code you write that uses the `ctx` context variable is trivially
unit tested with different feature gate settings. You should make sure
to unit test your code both with and without a feature gate enabled to
make sure it's properly guarded. See the following for an example of a
unit test that sets the feature gate to test behaviour:

```go
featureFlags, err := config.NewFeatureFlagsFromMap(map[string]string{
        "enable-api-fields": "alpha",
})
if err != nil {
	t.Fatalf("unexpected error initializing feature flags: %v", err)
}
cfg := &config.Config{
        FeatureFlags: featureFlags,
}
ctx := config.ToContext(context.Background(), cfg)
if err := ts.TestThing(ctx); err != nil {
	t.Errorf("unexpected error with alpha feature gate enabled: %v", err)
}
```

### Example YAMLs

Writing new YAML examples that require a feature gate to be set is easy.
New YAML example files typically go in a directory called something like
`examples/v1beta1/taskruns` in the root of the repo. To create a YAML that
should only be exercised when the `enable-api-fields` flag is `alpha` just
put it in an `alpha` subdirectory so the structure looks like:

```
examples/v1beta1/taskruns/alpha/your-example.yaml
```

This should work for both taskruns and pipelineruns.

**Note**: To execute alpha examples with the integration test runner you
must manually set the `enable-api-fields` feature flag to `alpha` in your
testing cluster before kicking off the tests.

When you set this flag to `stable` in your cluster it will prevent
`alpha` examples from being created by the test runner. When you set
the flag to `alpha` all examples are run, since we want to exercise
backwards-compatibility of the examples under alpha conditions.

### Integration Tests

For integration tests we provide the [`requireAnyGate` function](../../test/gate.go) which
should be passed to the `setup` function used by tests:

```go
c, namespace := setup(ctx, t, requireAnyGate(map[string]string{"enable-api-fields": "alpha"}))
```

This will Skip your integration test if the feature gate is not set to `alpha`
with a clear message explaining why it was skipped.

**Note**: As with running example YAMLs you have to manually set the `enable-api-fields`
flag to `alpha` in your test cluster to see your alpha integration tests
run. When the flag in your cluster is `alpha` _all_ integration tests are executed,
both `stable` and `alpha`. Setting the feature flag to `stable` will exclude `alpha`
tests.
