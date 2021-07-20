# Developing

## Getting started

First, you may want to [Ramp up](#ramp-up) on Kubernetes and Custom Resource Definitions (CRDs) as Tekton implements several Kubernetes resource controllers configured by Tekton CRDs. Then follow these steps to start developing and contributing project code:

1. [Setting up a development environment](#setting-up-a-environment-setup)
    1. [Setup a GitHub account accessible via SSH](#setup-a-github-account-accessible-via-ssh)
    1. [Install tools](#install-tools)
    1. [Configure environment](#configure-environment)
    1. [Setup a fork](#setup-a-fork) of the [tektoncd/pipeline](https://github.com/tektoncd/pipeline) project repository
    1. [Configure a container image registry](#configure-container-registry)
1. [Building and deploying](#building-and-deploying) Tekton source code from a local clone.
    1. [Setup a Kubernetes cluster](#setup-a-kubernetes-cluster)
    1. [Configure kubectl to use your cluster](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/)
    1. [Set up a docker repository 'ko' can push images to](https://github.com/knative/serving/blob/4a8c859741a4454bdd62c2b60069b7d05f5468e7/docs/setting-up-a-docker-registry.md)
1. [Developing and testing](#developing-and-testing) Tekton pipelines
    1. Learn how to [iterate](#iterating) on code changes
    1. [Managing Tekton Objects using `ko`](#managing-tekton-objects-using-ko) in Kubernetes
    1. [Standing up a K8s cluster with Tekton using the kind tool](#standing-up-a-k8s-cluster-with-tekton-using-the-kind-tool)
    1. [Accessing logs](#accessing-logs)
    1. [Adding new CRD types](#adding-new-crd-types)

---

### Ramp up

Welcome to the project! :clap::clap::clap:  You may find these resources helpful to "ramp up" on some of the technologies this project builds and runs on. This project extends Kubernetes (aka
`k8s`) with Custom Resource Definitions (CRDs). To find out more, read:

-   [The Kubernetes docs on Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) -
    These will orient you on what words like "Resource" and "Controller"
    concretely mean
-   [Understanding Kubernetes objects](https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/) -
    This will further solidify k8s nomenclature
-   [API conventions - Types(kinds)](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#types-kinds) -
    Another useful set of words describing words. "Objects" and "Lists" in k8s
    land
-   [Extend the Kubernetes API with CustomResourceDefinitions](https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/)-
    A tutorial demonstrating how a Custom Resource Definition can be added to
    Kubernetes without anything actually "happening" beyond being able to list
    Objects of that kind

At this point, you may find it useful to return to these `Tekton Pipeline` docs:

-   [Tekton Pipeline README](https://github.com/tektoncd/pipeline/blob/main/docs/README.md) -
    Some of the terms here may make more sense!
-   Install via
    [official installation docs](https://github.com/tektoncd/pipeline/blob/main/docs/install.md)
    or continue through [getting started for development](#getting-started)
-   [Tekton Pipeline "Hello World" tutorial](https://github.com/tektoncd/pipeline/blob/main/docs/tutorial.md) -
    Define `Tasks`, `Pipelines`, and `PipelineResources` (i.e., Tekton CRDs), and see what happens when they are run

---

## Setting up a development environment

### Setup a GitHub account accessible via SSH

GitHub is used for project Source Code Management (SCM) using the SSH protocol for authentication.

1. Create [a GitHub account](https://github.com/join) if you do not already have one.
1. Setup
[GitHub access via SSH](https://help.github.com/articles/connecting-to-github-with-ssh/)

### Install tools

You must install these tools:

1. [`git`](https://help.github.com/articles/set-up-git/): For source control

1. [`go`](https://golang.org/doc/install): The language Tekton Pipelines is
    built in.
    > **Note** Golang [version v1.15](https://golang.org/dl/) or higher is recommended.

1. [`ko`](https://github.com/google/ko#install): The Tekton project  uses `ko` to simplify the building of its container images from `go` source, push these images to the configured image repository and deploy these images into Kubernetes clusters.
    > **Note** `ko` version v0.5.1 or
    higher is required for `pipeline` to work correctly.

1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/): For
    interacting with your Kubernetes cluster

    > :warning: The user interacting with your K8s cluster **must be a cluster admin** to create role bindings.

    **Google Cloud Platform (GCP) example**:

    ```shell
    # Using gcloud to get your current user
    USER=$(gcloud config get-value core/account)
    # Make that user a cluster admin
    kubectl create clusterrolebinding cluster-admin-binding \
    --clusterrole=cluster-admin \
    --user="${USER}"
    ```

### Configure environment

To [build, deploy and run your Tekton Objects with `ko`](#install-pipeline), you'll need to set these environment variables:

1. `GOROOT`: Set `GOROOT` to the location of the Go installation you want `ko` to use for builds.

    > **Note**: You may need to set `GOROOT` if you installed Go tools to a non-default location or have multiple Go versions installed.

    If it is not set, `ko` infers the location by effectively using `go env GOROOT`.

1. `KO_DOCKER_REPO`: The docker repository to which developer images should be pushed.
For example:
    - Using **Google Container Registry (GCR)**:

        ```shell
        # format: `gcr.io/${GCP-PROJECT-NAME}`
        export KO_DOCKER_REPO='gcr.io/my-gcloud-project-name'
        ```

    - Using **Docker Desktop** (Docker Hub):

        ```shell
        # format: 'docker.io/${DOCKER_HUB_USERNAME}'
        export KO_DOCKER_REPO='docker.io/my-dockerhub-username'
        ```

    - You can also [host your own Docker Registry server](https://docs.docker.com/registry/deploying/) and reference it:

        ```shell
        # format: ${localhost:port}/{}
        export KO_DOCKER_REPO=`localhost:5000/mypipelineimages`
        ```

1. Optionally, add `$HOME/go/bin` to your system `PATH` so that any tooling installed via `go get` will work properly. For example:

    ```shell
    export PATH="${PATH}:$HOME/go/bin"
    ```

> **Note**: It is recommended to add these environment variables to your shell's configuration files (e.g., `~/.bash_profile` or `~/.bashrc`).

### Setup a fork

The Tekton project requires that you develop (commit) code changes to branches that belong to a fork of the `tektoncd/pipeline` repository in your GitHub account before submitting them as Pull Requests (PRs) to the actual project repository.

1. [Create a fork](https://help.github.com/articles/fork-a-repo/) of the `tektoncd/pipeline` repository in your GitHub account.

1. Create a clone of your fork on your local machine:

    ```shell
    git clone git@github.com:${YOUR_GITHUB_USERNAME}/pipeline.git
    ```

    > **Note**: Tekton uses [Go Modules](https://golang.org/doc/modules/gomod-ref) (i.e., `go mod`) for package management so you may clone the repository to a location of your choosing.

1. Configure `git` remote repositories

    Adding `tektoncd/pipelines` as the `upstream` and your fork as the `origin` remote repositories to your `.git/config` sets you up nicely for regularly [syncing your fork](https://help.github.com/articles/syncing-a-fork/) and submitting pull requests.

    1. Change into the project directory

        ```shell
        cd pipeline
        ```

    1. Configure Tekton as the `upstream` repository

        ```shell
        git remote add upstream git@github.com:tektoncd/pipeline.git

        # Optional: Prevent accidental pushing of commits by changing the upstream URL to `no_push`
        git remote set-url --push upstream no_push
        ```

    1. Configure your fork as the `origin` repository

        ```shell
        git remote add origin git@github.com:${YOUR_GITHUB_USERNAME}/pipeline.git
        ```

### Configure Container Registry

Depending on your chosen container registry that you set in the `KO_DOCKER_REPO` environment variable, you may need to additionally configure access control to allow `ko` to authenticate to it.

<!-- TODO: Need instructions for MiniKube -->

#### Using Docker Desktop (Docker Hub)

Docker Desktop provides seamless integration with both a local (default) image registry as well as Docker Hub remote registries.  To use Docker Hub registries with `ko`, all you need do is to configure Docker Desktop with your Docker ID and password in its dashboard.

#### Using Google Container Registry (GCR)

If using GCR with `ko`, make sure to configure
[authentication](https://cloud.google.com/container-registry/docs/advanced-authentication#standalone_docker_credential_helper)
for your `KO_DOCKER_REPO` if required. To be able to push images to
`gcr.io/<project>`, you need to run this once:

```shell
gcloud auth configure-docker
```

Besides, if your registry `KO_DOCKER_REPO` is private, then you also should create a [secret](https://kubernetes.io/docs/concepts/configuration/secret/#docker-config-secrets)
and add it to `tekton-pipelines-controller` and `tekton-pipelines-webhook` ServiceAccounts accordingly to pull images.

1. Create a secret

    ```yaml
    kubectl create secret docker-registry ${SECRET_NAME} \
    --docker-username=${USERNAME} \
    --docker-password=${PASSWORD} \
    --docker-email=me@here.com \
    --namespace=tekton-pipelines
    ```

2. Add the `secret` to the K8s `ServiceAccount` configuration file

    Before using `ko` to deploy `tekton-pipelines`, you need to change [the contents of the ServiceAccount](./config/200-serviceaccount.yaml) configuration file as shown below:

    ```yaml
    apiVersion: v1
    kind: ServiceAccount
    metadata:
    name: tekton-pipelines-controller
    namespace: tekton-pipelines
    labels:
        app.kubernetes.io/component: controller
        app.kubernetes.io/instance: default
        app.kubernetes.io/part-of: tekton-pipelines
    imagePullSecrets:
    - name: ${SECRET_NAME}
    ---
    apiVersion: v1
    kind: ServiceAccount
    metadata:
    name: tekton-pipelines-webhook
    namespace: tekton-pipelines
    labels:
        app.kubernetes.io/component: webhook
        app.kubernetes.io/instance: default
        app.kubernetes.io/part-of: tekton-pipelines
    imagePullSecrets:
    - name: ${SECRET_NAME}
    ```

---

## Building and deploying

### Setup a Kubernetes cluster

The recommended minimum development configuration is:

- Kubernetes version 1.18 or later
- 4 (virtual) CPU nodes
  - 8 GB of (actual or virtualized) platform memory
- Node autoscaling, up to 3 nodes

#### Using MiniKube

- Follow the instructions for [running locally with Minikube](docs/developers/local-setup.md#using-minikube)

#### Using Docker Desktop

- Follow the instructions for [running locally with Docker Desktop](docs/developers/local-setup.md#using-docker-desktop)

#### Using GKE

1. [Install required tools and setup GCP project](https://knative.dev/v0.12-docs/install/knative-with-gke/)
    (You may find it useful to save the ID of the project in an environment
    variable (e.g. `PROJECT_ID`).

<!-- TODO: Someone needs to validate the cluster-version-->
1. Create a GKE cluster (with `--cluster-version=latest` but you can use any
    version 1.18 or later):

    ```bash
    export PROJECT_ID=my-gcp-project
    export CLUSTER_NAME=mycoolcluster

    gcloud container clusters create $CLUSTER_NAME \
     --enable-autoscaling \
     --min-nodes=1 \
     --max-nodes=3 \
     --scopes=cloud-platform \
     --enable-basic-auth \
     --no-issue-client-certificate \
     --project=$PROJECT_ID \
     --region=us-central1 \
     --machine-type=n1-standard-4 \
     --image-type=cos \
     --num-nodes=1 \
     --cluster-version=1.18
    ```

    > **Note**: The recommended [GCE machine type](https://cloud.google.com/compute/docs/machine-types) is `'n1-standard-4'`.

    > **Note**: [The `'--scopes'` argument](https://cloud.google.com/sdk/gcloud/reference/container/clusters/create#--scopes) on the  `'gcloud container cluster create'` command controls what GCP resources the cluster's default service account has access to; for example, to give the default service account full access to your GCR registry, you can add `'storage-full'` to the `--scopes` arg. See [Authenticating to GCP](https://cloud.google.com/kubernetes-engine/docs/tutorials/authenticating-to-cloud-platform) for more details.

1. Grant cluster-admin permissions to the current user:

   ```bash
   kubectl create clusterrolebinding cluster-admin-binding \
   --clusterrole=cluster-admin \
   --user=$(gcloud config get-value core/account)
   ```

---

## Developing and testing

### Iterating on code changes

While iterating on code changes to the project, you may need to:

1. [Manage Tekton objects](#managing-tekton-objects-using-ko)
1. [Verify installation](#verify-installation) and make sure there are no errors by [accessing the logs](#accessing-logs)
1. Use various development scripts, as needed, in the ['hack' directory](https://github.com/tektoncd/pipeline/tree/main/hack), For example:
    - Update your (external) dependencies with: `./hack/update-deps.sh`
    - Update your type definitions with: `./hack/update-codegen.sh`
    -  Update your OpenAPI specs with: `./hack/update-openapigen.sh`
1. Update or [add new CRD types](#adding-new-types) as needed
1. Update, [add and run tests](./test/README.md#tests)

To make changes to these CRDs, you will probably interact with:

- The CRD type definitions in [./pkg/apis/pipeline/v1beta1](./pkg/apis/pipeline/v1beta1)
- The reconcilers in [./pkg/reconciler](./pkg/reconciler)
- The clients are in [./pkg/client](./pkg/client) (these are generated by
    `./hack/update-codegen.sh`)

---

### Managing Tekton Objects using `ko`

The `ko` command is the preferred method to manage (i.e., create, modify or delete) Tekton Objects in Kubernetes from your local fork of the project. Some common operations include:

#### Install Pipeline

You can stand up a version of Tekton using your local clone's code to the currently configured K8s context (i.e.,  `kubectl config current-context`):

```shell
ko apply -f config/
```

#### Verify installation

You can verify your development installation using `ko` was successful by checking to see oif the Tekton pipeline pods are running in Kubernetes:

```shell
kubectl get pods -n tekton-pipelines
```

#### Delete Pipeline

You can clean up everything with:

```shell
# If you should not delete the namespace of a pipeline component
ko delete -f config/

# If you also can delete the namespace of a pipeline component
ko delete -R -f config/
```

**Note:** If you use a pipeline component in the same namespace as other components such as dashboard or triggers, executing `ko delete -R -f config/` deletes these other components too.

#### Redeploy controller

As you make changes to the code, you can redeploy your controller with:

```shell
ko apply -f config/controller.yaml
```

#### Installing into custom namespaces

When managing different development branches of code (with changed Tekton objects and controllers) in the same K8s instance, it may be helpful to install them into a custom (non-default) namespace. The ability to map a code branch to a corresponding namespace may make it easier to identify and manage the objects as a group as well as isolate log output.

To install into a different namespace you can use this script:

```bash
#!/usr/bin/env bash
set -e

# Set your target namespace here
TARGET_NAMESPACE=new-target-namespace

ko resolve -f config | sed -e '/kind: Namespace/!b;n;n;s/:.*/: '"${TARGET_NAMESPACE}"'/' | \
    sed "s/namespace: tekton-pipelines$/namespace: ${TARGET_NAMESPACE}/" | \
    kubectl apply -f-
kubectl set env deployments --all SYSTEM_NAMESPACE=${TARGET_NAMESPACE} -n ${TARGET_NAMESPACE}
```

This script will cause `ko` to:

- Change (resolve) all `namespace` values in K8s configuration files within the `config/` subdirectory to be updated to a name of your choosing.
- Builds and push images with the new namespace to your container registry and
- Update all Tekton Objects in K8s using these images

It will also update the default system namespace used for K8s `deployments` to the new value for all subsequent `kubectl` commands.

---

### Standing up a K8s cluster with Tekton using the `kind` tool

An alternative to standing up your own K8s cluster and installing Tekton using `ko` is by using the [kind](https://kind.sigs.k8s.io/) tool. It was designed to help create and run local Kubernetes clusters in Docker to assist in local development and testing.

The [Tekton "plumbing" project](https://github.com/tektoncd/plumbing) provides a convenience script, named ['tekton_in_kind.sh'](https://github.com/tektoncd/plumbing/blob/main/hack/tekton_in_kind.sh), that leverages `kind` to create a cluster and then deploy Tekton Pipeline, [Tekton Triggers](https://github.com/tektoncd/triggers) and [Tekton Dashboard](https://github.com/tektoncd/dashboard) components into it.

See Tekton Plumbing's [DEVELOPMENT.md](https://github.com/tektoncd/plumbing/blob/main/DEVELOPMENT.md) for more details on this and other helpful scripts and tools.

#### Installation and prerequisites

- Clone the [Tekton Plumbing](https://github.com/tektoncd/plumbing) repository which has the ['tekton_in_kind.sh'](https://github.com/tektoncd/plumbing/blob/main/hack/tekton_in_kind.sh) and other helpful scripts.
- `kind`: Install using its ["quick start"](https://kind.sigs.k8s.io/docs/user/quick-start/) documentation.
- `Docker`: `kind` also requires Docker to be installed and running locally. Use the [Get Docker](https://docs.docker.com/get-docker/) instructions.

#### Create a cluster with Tekton components

Change into the Tekton plumbing repository you cloned and invoke the script:

```shell
cd plumbing
./hack/tekton_in_kind.sh
```

The script, after using `kind` to create the K8s cluster, uses `kubectl` to install the `latest` released versions of Tekton components. After successful completion, the script will have:

- Created a K8s cluster named `tekton`
- Created a `cluster-context` for `kubectl` named `'kind-tekton'` and set it as the `current-context`
- Deployed the latest Tekton Pipeline, Trigger and Dashboard components
- Made Tekton Dashboard available at `http://localhost:9097`

After the Tekton components are installed using this script, you can than use the `ko` tool to build and apply your development changes to the cluster for testing.

##### Using a different cluster name and Tekton versions

You can also specify a different cluster name and released versions of components you want installed:

```shell
# Script usage:
tekton_in_kind.sh [-c cluster-name -p pipeline-version -t triggers-version -d dashboard-version]"
```

#### Delete the cluster

If you wish to delete the cluster that the script created, use the following command:

```shell
# Delete the cluster using the default name
kind delete cluster --name tekton
```

> **Note**: Be sure to set a `kubectl` context, as its unset as a side-effect of the delete (i.e., `kubectl config use-context`*`<context-name>`*).

---

### Accessing logs

To look at the controller logs, run:

```shell
kubectl -n tekton-pipelines logs $(kubectl -n tekton-pipelines get pods -l app=tekton-pipelines-controller -o name)
```

To look at the webhook logs, run:

```shell
kubectl -n tekton-pipelines logs $(kubectl -n tekton-pipelines get pods -l app=tekton-pipelines-webhook -o name)
```

To look at the logs for individual `TaskRuns` or `PipelineRuns`, see
[docs on accessing logs](docs/logs.md).

### Adding new CRD types

If you need to add a new CRD type, you will need to add:

1. A yaml definition in [config/](./config)
1. Add the type to the cluster roles in:
    - [200-clusterrole.yaml](./config/200-clusterrole.yaml)
    - [clusterrole-aggregate-edit.yaml](./config/clusterrole-aggregate-edit.yaml)
    - [clusterrole-aggregate-view.yaml](./config/clusterrole-aggregate-view.yaml)
1. Add go structs for the types in
    [pkg/apis/pipeline/v1alpha1](./pkg/apis/pipeline/v1alpha1) e.g
    [condition_types.go](./pkg/apis/pipeline/v1alpha1/condition_types.go) This
    should implement the
    [Defaultable](./pkg/apis/pipeline/v1alpha1/condition_defaults.go) and
    [Validatable](./pkg/apis/pipeline/v1alpha1/condition_validation.go)
    interfaces as they are needed for the webhook in the next step.
1. Register it with the [webhook](./cmd/webhook/main.go)
1. Add the new type to the
    [list of known types](./pkg/apis/pipeline/v1alpha1/register.go)

_See [the API compatibility policy](api_compatibility_policy.md)._
