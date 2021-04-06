<!--
---
linkTitle: "Step Composition Contract"
weight: 8
---
-->

# Tekton Step Composition v0.1

This document outlines the step composition model.

## Aims

* Allow Tasks and Steps to be used from versioned files in git repositories, OCI bundles or kubernetes resources
* Add local steps before/after/in between the steps from a shared Task
* Override properties on an inherited step such as changing the image, command, args, environment variables, volumes, script.
* Try be fairly DRY yet simple so that its immediately obvious looking at a Tekton YAML what it means. e.g. default to inheriting tasks from github so that the [tekton catalog](https://github.com/tektoncd/catalog) tasks can be used in a nice concise way
    
## Using steps

To use steps from a `Task`, `TaskRun`, `Pipeline` or `PipelineRun` you use the `uses:` property on your step.

The `uses:` object supports a number of different remote locations

### git resources

To reuse steps from a `Task`, `TaskRun`, `Pipeline` or `PipelineRun` in a git repository use the `git:` property which uses a git URI notation:

```yaml
- uses: 
    git: owner/repository/pathToFile@branchTagOrSHA
```
                                                     

#### GitHub URIs

The most common place of reusing tasks from git will be [GitHub](https://github.com/) which is where most open source software lives including the [Tekton Catalog](https://github.com/tektoncd/catalog) so to keep the Tekton YAML concise we support a github specific notation of:

```
ownerOrUser/repositoryName/pathToResource[@branchTagOrSHA]
```
                                                                                              
e.g. to reference a Tekton catalog task use: [tektoncd/catalog/task/kaniko/0.1/kaniko.yaml](https://github.com/tektoncd/catalog/task/kaniko/0.1/kaniko.yaml) which will use the latest version of `0.1` of the `kaniko` task

#### Other git servers

For other git servers we support the following syntax where the `gitCloneURI` ends with `.git`

```
gitCloneURI/pathToResource[@branchTagOrSHA]
```
                     
So the following examples can be used:
    
* `https://github.com/myowner/myrepo.git/some/path.yaml`
* `git@github.com:bar/foo.git/thingy.yaml@mybranch`  
* `https://mybitbucket.com/scm/myowner/myrepo.git/thingy.yaml@mybranch`
* `git://host.xz/org/repo.git/some/path.yaml@v1.2.3`

### kubernetes resources

To reference a `Task` in kubernetes use the usual `TaskRef` notation...

```yaml
- uses: 
    ref: 
      name: my-task
```

### OCI bundles

To reference a `Task` in an OCI bundle use the usual `TaskRef` notation...

```yaml
- uses: 
    ref: 
      bundle: gcr.io/myproject/mybundle:1.2.3
      name: my-task
```



## Reusing all steps of a Task

To reuse all the steps of a Task inside your task add a `uses:`  to a step [like this example](../pkg/stepper/test_data/tests/task/input.yaml) 

```yaml 
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  generateName: my-pipeline-
spec:
  steps:
  - uses:
      git: tektoncd/catalog/task/kaniko/0.1/kaniko.yaml
  - name: my-custom-step
    image: golang:1.15
    script: |
      script: |
        #!/bin/sh
        echo "hello world"
```

This will then reuse all of the steps in the [tektoncd/catalog/task/kaniko/0.1/kaniko.yaml](https://github.com/tektoncd/catalog/task/kaniko/0.1/kaniko.yaml) file in place inside your `Task` before the **my-custom-step**

The [generated Task is here](../pkg/stepper/test_data/tests/task/expected.yaml)

### Customising all reused steps

You may want to add some common environment variables or volume mounts to the used steps. You can do this by adding the `env`, `envFrom` or `volumeMount` properties to your step [like this example](../pkg/stepper/test_data/tests/task_override_all_steps/input.yaml)

e.g. lets add a **MY_VAR** environment variable to all reused steps:


```yaml 
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  generateName: my-pipeline-
spec:
  steps:
  - uses:
      git: tektoncd/catalog/task/kaniko/0.1/kaniko.yaml
    env:
      - name: MY_VAR
        value: CHEESE
  - name: my-custom-step
    image: golang:1.15
    script: |
      #!/bin/sh
      echo "hello world"
```

The [generated Task is here](../pkg/stepper/test_data/tests/task_override_all_steps/expected.yaml)


## Reusing individual steps in a Task

You can `use:` each step in a Task individually  [like this example](../pkg/stepper/test_data/tests/task_inline_steps/input.yaml) 

```yaml 
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  generateName: my-pipeline-
spec:
  steps:
  - uses:
      git: tektoncd/catalog/task/kaniko/0.1/kaniko.yaml
      step: build-and-push
  - uses:
      git: tektoncd/catalog/task/kaniko/0.1/kaniko.yaml
      step: write-digest
  - uses:
      git: tektoncd/catalog/task/kaniko/0.1/kaniko.yaml
      step: digest-to-results
```
   

The [generated Task is here](../pkg/stepper/test_data/tests/task_inline_steps/expected.yaml)


This is more verbose than including all of the steps in a single step but provides a number of additional capabilities:

* override anything in each step such as its `command`, `args`, `script` in addition to customising each step differently
* add your own steps before/after the reused steps

You can see that being used in the next example...

### Customising individually reused steps

You may want to customize specific steps. In [this example](../pkg/stepper/test_data/tests/task_override_steps/input.yaml)  we will: 

* adding an extra **MY_VAR** environment variable to the **write-digest** step
* adding an extra step **my-custom-step** before the reused **digest-to-results** step 
 
 ```yaml 
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  generateName: my-pipeline-
spec:
  steps:
  - uses:
      git: tektoncd/catalog/task/kaniko/0.1/kaniko.yaml
      step: build-and-push
  - uses:
      git: tektoncd/catalog/task/kaniko/0.1/kaniko.yaml
      step: write-digest
    env:
      - name: MY_VAR
        value: CHEESE
  - name: my-custom-step
    image: golang:1.15
    script: |
      #!/bin/sh
      echo "hello world"
  - uses:
      git: tektoncd/catalog/task/kaniko/0.1/kaniko.yaml
      step: digest-to-results
```
 
The [generated Task is here](../pkg/stepper/test_data/tests/task_override_steps/expected.yaml)
