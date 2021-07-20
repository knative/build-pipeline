<!--
---
linkTitle: "Variable Substitutions"
weight: 900
---
-->
# Variable Substitutions Supported by `Tasks` and `Pipelines`

This page documents the variable substitutions supported by `Tasks` and `Pipelines`.

For instructions on using variable substitutions see the relevant section of [the Tasks doc](tasks.md#using-variable-substitution).

**Note:** Tekton does not escape the contents of variables. Task authors are responsible for properly escaping a variable's value according to the shell, image or scripting language that the variable will be used in.

## Variables available in a `Pipeline`

| Variable | Description |
| -------- | ----------- |
| `params.<param name>` | The value of the parameter at runtime. |
| `tasks.<taskName>.results.<resultName>` | The value of the `Task's` result. Can alter `Task` execution order within a `Pipeline`.) |
| `workspaces.<workspaceName>.bound` | Whether a `Workspace` has been bound or not. "false" if the `Workspace` declaration has `optional: true` and the Workspace binding was omitted by the PipelineRun. |
| `context.pipelineRun.name` | The name of the `PipelineRun` that this `Pipeline` is running in. |
| `context.pipelineRun.namespace` | The namespace of the `PipelineRun` that this `Pipeline` is running in. |
| `context.pipelineRun.uid` | The uid of the `PipelineRun` that this `Pipeline` is running in. |
| `context.pipeline.name` | The name of this `Pipeline` . |
| `tasks.<pipelineTaskName>.status` | The execution status of the specified `pipelineTask`, only available in `finally` tasks. The execution status can be set to any one of the values (`Succeeded`, `Failed`, or `None`) described [here](pipelines.md#using-execution-status-of-pipelinetask)|
| `tasks.status` | An aggregate status of all the `pipelineTasks` under the `tasks` section (excluding the `finally` section). This variable is only available in the `finally` tasks and can have any one of the values (`Succeeded`, `Failed`, `Completed`, or `None`) described [here](pipelines.md#using-aggregate-execution-status-of-all-tasks).  |
| `context.pipelineTask.retries` | The retries of this `PipelineTask`. |

## Variables available in a `Task`

| Variable | Description |
| -------- | ----------- |
| `params.<param name>` | The value of the parameter at runtime. |
| `resources.inputs.<resourceName>.path` | The path to the input resource's directory. |
| `resources.outputs.<resourceName>.path` | The path to the output resource's directory. |
| `results.<resultName>.path` | The path to the file where the `Task` writes its results data. |
| `workspaces.<workspaceName>.path` | The path to the mounted `Workspace`. Empty string if an optional `Workspace` has not been provided by the TaskRun. |
| `workspaces.<workspaceName>.bound` | Whether a `Workspace` has been bound or not. "false" if an optional`Workspace` has not been provided by the TaskRun. |
| `workspaces.<workspaceName>.claim` | The name of the `PersistentVolumeClaim` specified as a volume source for the `Workspace`. Empty string for other volume types. |
| `workspaces.<workspaceName>.volume` | The name of the volume populating the `Workspace`. |
| `credentials.path` | The path to credentials injected from Secrets with matching annotations. |
| `context.taskRun.name` | The name of the `TaskRun` that this `Task` is running in. |
| `context.taskRun.namespace` | The namespace of the `TaskRun` that this `Task` is running in. |
| `context.taskRun.uid` | The uid of the `TaskRun` that this `Task` is running in. |
| `context.task.name` | The name of this `Task`. |
| `context.task.retry-count` | The current retry number of this `Task`. |

### `PipelineResource` variables available in a `Task`

Each supported type of `PipelineResource` specified within a `Task` exposes a unique set
of variables. This section lists the variables exposed by each type. You can access a
variable via `resources.inputs.<resourceName>.<variableName>` or
`resources.outputs.<resourceName>.<variableName>`.

#### Variables for the `Git` type

| Variable | Description |
| -------- | ----------- |
| `name` | The name of the resource. |
| `type` | Type value of `"git"`. |
| `url` | The URL of the Git repository. |
| `revision` | The revision to check out. |
| `refspec` | The value of the resource's `refspec` parameter. |
| `depth` | The integer value of the resource's `depth` parameter. |
| `sslVerify` | The value of the resource's `sslVerify` parameter, either `"true"` or `"false"`. |
| `httpProxy` | The value of the resource's `httpProxy` parameter. |
| `httpsProxy` | The value of the resource's `httpsProxy` parameter. |
| `noProxy` | The value of the resource's `noProxy` parameter. |

#### Variables for the `PullRequest` type

| Variable | Description |
| -------- | ----------- |
| `name` | The name of the resource. |
| `type` | Type value of `"pullRequest"`.|
| `url` | The URL of the pull request. |
| `provider` | Provider value, either `"github"` or `"gitlab"`. |
| `insecure-skip-tls-verify` | The value of the resource's `insecure-skip-tls-verify` parameter, either `"true"` or `"false"`. |

#### Variables for the `Image` type

| Variable | Description |
| -------- | ----------- |
| `name` | The name of the resource. |
| `type` | Type value of `"image"`. |
| `url` | The complete path to the image. |
| `digest` | The digest of the image. |

#### Variables for the `GCS` type

| Variable | Description |
| -------- | ----------- |
| `name` | The name of the resource. |
| `type` | Type value of `"gcs"`. |
| `location` | The fully qualified address of the blob storage. |

#### Variables for the `Cluster` type

| Variable | Description |
| -------- | ----------- |
| `name` | The name of the resource. |
| `type` | Type value of `"cluster"`. |
| `url` | Host URL of the master node. |
| `username` | The user with access to the cluster. |
| `password` | The password for the user specified in `username`. |
| `namespace` | The namespace to target in the cluster. |
| `token` | The bearer token. |
| `insecure` | Whether to verify the TLS connection to the server, either `"true"` or `"false"`. |
| `cadata` | Stringified PEM-encoded bytes from the relevant root certificate bundle. |
| `clientKeyData` | Stringified PEM-encoded bytes from the client key file for TLS. |
| `clientCertificateData` | Stringified PEM-encoded bytes from the client certificate file for TLS. |

#### Variables for the `CloudEvent` type

| Variable | Description |
| -------- | ----------- |
| `name` | The name of the resource. |
| `type` | Type value of `"cloudEvent"`. |
| `target-uri` | The URI to hit with cloud event payloads. |

## Fields that accept variable substitutions

| CRD | Field |
| --- | ----- |
| `Task` | `spec.steps[].name` |
| `Task` | `spec.steps[].image` |
| `Task` | `spec.steps[].imagePullPolicy` |
| `Task` | `spec.steps[].env.value` |
| `Task` | `spec.steps[].env.valuefrom.secretkeyref.name` |
| `Task` | `spec.steps[].env.valuefrom.secretkeyref.key` |
| `Task` | `spec.steps[].env.valuefrom.configmapkeyref.name` |
| `Task` | `spec.steps[].env.valuefrom.configmapkeyref.key` |
| `Task` | `spec.steps[].volumemounts.name` |
| `Task` | `spec.steps[].volumemounts.mountpath` |
| `Task` | `spec.steps[].volumemounts.subpath` |
| `Task` | `spec.volumes[].name` |
| `Task` | `spec.volumes[].configmap.name` |
| `Task` | `spec.volumes[].configmap.items[].key` |
| `Task` | `spec.volumes[].configmap.items[].path` |
| `Task` | `spec.volumes[].secret.secretname` |
| `Task` | `spec.volumes[].secret.items[].key` |
| `Task` | `spec.volumes[].secret.items[].path` |
| `Task` | `spec.volumes[].persistentvolumeclaim.claimname` |
| `Task` | `spec.volumes[].projected.sources.configmap.name` |
| `Task` | `spec.volumes[].projected.sources.secret.name` |
| `Task` | `spec.volumes[].projected.sources.serviceaccounttoken.audience` |
| `Task` | `spec.volumes[].csi.nodepublishsecretref.name` |
| `Task` | `spec.volumes[].csi.volumeattributes.* `|
| `Task` | `spec.sidecars[].name` |
| `Task` | `spec.sidecars[].image` |
| `Task` | `spec.sidecars[].imagePullPolicy` |
| `Task` | `spec.sidecars[].env.value` |
| `Task` | `spec.sidecars[].env.valuefrom.secretkeyref.name` |
| `Task` | `spec.sidecars[].env.valuefrom.secretkeyref.key` |
| `Task` | `spec.sidecars[].env.valuefrom.configmapkeyref.name` |
| `Task` | `spec.sidecars[].env.valuefrom.configmapkeyref.key` |
| `Task` | `spec.sidecars[].volumemounts.name` |
| `Task` | `spec.sidecars[].volumemounts.mountpath` |
| `Task` | `spec.sidecars[].volumemounts.subpath` |
| `Task` | `spec.sidecars[].script` |
| `Task` | `spec.workspaces[].mountPath` |
| `Pipeline` | `spec.tasks[].params[].value` |
| `Pipeline` | `spec.tasks[].conditions[].params[].value` |
| `Pipeline` | `spec.results[].value` |
| `Pipeline` | `spec.tasks[].when[].input` |
| `Pipeline` | `spec.tasks[].when[].values` |