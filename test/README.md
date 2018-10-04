# Tests

## Unit tests

Unit tests live side by side with the code they are testing and can be run with:

```shell
go test ./...
```

_By default `go test` will not run [the integration tests](#integration-tests), which need
`-tags=e2e` to be enabled._

### Unit testing Controllers
Kubernetes client-go provides a number of fake clients and objects for unit testing. The ones we will be using are:

1. [fake kubernetes client](k8s.io/client-go/kubernetes/fake): Provides a fake REST interface to interact with Kubernetes API
1. [fake pipeline client](./../pkg/client/clientset/versioned/fake/clientset_generated.go) : Provides a fake REST PipelineClient Interface to interact with Pipeline CRDs.

You can create a fake PipelineClient for the Controller under test like [this](./../pkg/reconciler/v1alpha1/pipelinerun/pipelinerun_test.go#L102).

This [pipelineClient](./../pkg/client/clientset/versioned/clientset.go#L34) is initialized with no runtime objects. You can also initialie the client with kubernetes objects and can interact with them using the `pipelineClient.Pipeline()`

```
 import (
     v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
 )

 obj := *v1alpha1.PipelineRun{
  ObjectMeta: metav1.ObjectMeta{
	Name:      "name",
	Namespace: "namespace",
  },
  Spec: v1alpha1.PipelineRunSpec{
	PipelineRef: v1alpha1.PipelineRef{
	  Name:       "test-pipeline",
	  APIVersion: "a1",
	},
}}
pipelineClient := fakepipelineclientset.NewSimpleClientset(obj)
objs := pipelineClient.Pipeline().PipelineRuns("namespace").List(v1.ListOptions{})
//You can verify if List was called in yout test like this
action :=  pipelineClient.Actions()[0]
if action.GetVerb() != "list" {
    t.Errorf("expected list to be called, found %s", action.GetVerb())
}
```

To test the Controller for crd objects, we need to add test crd objects to the [informers](./../pkg/client/informers)
so that the [listers](./../pkg/client/listers) can access these.

To add test `PipelineRun` objects to the listers, you can

```
pipelineClient := fakepipelineclientset.NewSimpleClientset()
sharedInfomer := informers.NewSharedInformerFactory(pipelineClient, 0)
pipelineRunsInformer := sharedInfomer.Pipeline().V1alpha1().PipelineRuns()

obj := *v1alpha1.PipelineRun{
  ObjectMeta: metav1.ObjectMeta{
	Name:      "name",
	Namespace: "namespace",
  },
  Spec: v1alpha1.PipelineRunSpec{
	PipelineRef: v1alpha1.PipelineRef{
	  Name:       "test-pipeline",
	  APIVersion: "a1",
	},
}}
pipelineRunsInformer.Informer().GetIndexer().Add(obj)
```

## Integration tests

Integration tests live in this directory. To run these tests, you must provide `go` with
`-tags=e2e`, and you may want to use some of the [common flags](#common-flags):

```shell
go test -v -count=1 -tags=e2e ./test
```

You can also use
[all of flags defined in `knative/pkg/test`](https://github.com/knative/pkg/tree/master/test#flags).

### Flags

* By default the e2e tests against the current cluster in `~/.kube/config`
  using the environment specified in [your environment variables](/DEVELOPMENT.md#environment-setup).
* Since these tests are fairly slow, running them with logging
  enabled is recommended (`-v`).
* Using [`--logverbose`](#output-verbose-log) to see the verbose log output from test as well as from k8s libraries.
* Using `-count=1` is [the idiomatic way to disable test caching](https://golang.org/doc/go1.10#test)

You can [use test flags](#flags) to control the environment
your tests run against, i.e. override [your environment variables](/DEVELOPMENT.md#environment-setup):

```bash
go test -v -tags=e2e -count=1 ./test --kubeconfig ~/special/kubeconfig --cluster myspecialcluster
```

Tests importing [`github.com/knative/build-pipline/test`](#adding-integration-tests) recognize these
[all flags added by `knative/pkg/test`](https://github.com/knative/pkg/tree/master/test#flags).

_Note the environment variable `K8S_CLUSTER_OVERRIDE`, while used by [knative/serving](https://github.com/knative/serving)
and not by this project, will override the cluster used by the integration tests since they use
[the same libs to get these flags](https://github.com/knative/serving)._

### Adding integration tests

In the [`test`](/test/) dir you will find several libraries in the `test` package
you can use in your tests.

This library exists partially in this directory and partially in
[`knative/pkg/test`](https://github.com/knative/pkg/tree/master/test).

The libs in this dir can:

* [`init.go`](./init.go) initializes anything needed globally be the tests
* [Get access to client objects](#get-access-to-client-objects)

All integration tests _must_ be marked with the `e2e` [build constraint](https://golang.org/pkg/go/build/)
so that `go test ./...` can be used to run only [the unit tests](#unit-tests), i.e.:

```go
// +build e2e
```

#### Get access to client objects

To initialize client objects use [the command line flags](#use-flags)
which describe the environment:

```go
func setup(t *testing.T) *test.Clients {
    clients, err := test.NewClients(kubeconfig, cluster, namespaceName)
    if err != nil {
        t.Fatalf("Couldn't initialize clients: %v", err)
    }
    return clients
}
```

The `Clients` struct contains initialized clients for accessing:

* Kubernetes objects
* [`Pipelines`](https://github.com/knative/build-pipeline#pipeline)
* TODO: incrementally add clients for other types

For example, to create a `Pipeline`:

```bash
_, err = clients.PipelineClient.Pipelines.Create(test.Route(namespaceName, pipelineName))
```

And you can use the client to clean up resources created by your test (e.g. in
[your test cleanup](https://github.com/knative/pkg/tree/master/test#ensure-test-cleanup)):

```go
func tearDown(clients *test.Clients) {
    if clients != nil {
        clients.Delete([]string{routeName}, []string{configName})
    }
}
```

_See [clients.go](./clients.go)._


#### Generate random names

You can use the function `AppendRandomString` to create random names for `crd`s or anything else,
so that your tests can use unique names each time they run.

```go
namespace := test.AppendRandomString('arendelle')
```

_See [randstring.go](./randstring.go)._

## Presubmit tests

[`presubmit-tests.sh`](./presubmit-tests.sh) is the entry point for all tests
[run on presubmit by Prow](../CONTRIBUTING.md#pull-request-process).

You can run this locally with:

```shell
test/presubmit-tests.sh
test/presubmit-tests.sh --build-tests
test/presubmit-tests.sh --unit-tests
```

Prow is configured in [the knative `config.yaml` in `knative/test-infra`](https://github.com/knative/test-infra/blob/master/ci/prow/config.yaml)
via the sections for `knative/build-pipeline`.

### Running presubmit integration tests

When run using Prow, integration tests will try to get a new cluster using [boskos](https://github.com/kubernetes/test-infra/tree/master/boskos) and
[these hardcoded GKE projects](https://github.com/knative/test-infra/blob/master/ci/prow/boskos/resources.yaml#L15),
which only [the `knative/test-infra` OWNERS](https://github.com/knative/test-infra/blob/master/OWNERS)
have access to.

If you would like to run the integration tests against your cluster, you can use the
`K8S_CLUSTER_OVERRIDE` environment variable to force the scripts to use your own cluster,
provide `KO_DOCKER_REPO` (as specified in the [DEVELOPMENT.md](../DEVELOPMENT.md#environment-setup)),
use `e2e-tests.sh` directly and provide the `--run-tests` argument:

```shell
export K8S_CLUSTER_OVERRIDE=my_k8s_cluster # corresponds to a `context` in your kubeconfig
export KO_DOCKER_REPO=gcr.io/my_docker_repo # required for deployments using `ko`
test/e2e-tests.sh --run-tests
```

Or you can set `$PROJECT_ID` to a GCP project and rely on
[kubetest](https://github.com/kubernetes/test-infra/tree/master/kubetest)
to setup a cluster for you:

```shell
export K8S_CLUSTER_OVERRIDE=
export PROJECT_ID=my_gcp_project
export KO_DOCKER_REPO=gcr.io/my_docker_repo # required for deployments using `ko`
test/presubmit-tests.sh --integration-tests
```
