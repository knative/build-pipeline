# Tests

## Unit tests

Unit tests live side by side with the code they are testing and can be run with:

```shell
go test ./...
```

_By default `go test` will not run [the integration tests](#integration-tests), which need
`-tags=e2e` to be enabled._

### Unit testing Controllers
Kubernetes client-go provides a umber of fake clients and objects for unit testing. The ones we will be using are:

1. [fake kubernetes client](k8s.io/client-go/kubernetes/fake): Provides a fake REST interface to interacti with Kubernetes API
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
//You can verify if List was called in yout test using
action :=  pipelineClient.Actions()[0]
if action.GetVerb() != "list" {
    t.Errorf("expected list to be called, found %s", action.GetVerb())
}
```

To test the Controller for crd objects, we need to provide mocks for [listers](./../pkg/client/listers). If mocks are not provider, the lister will return empty objects.
The Controller does not use the pipelineClient to list  CRDs. To mock listers, we need to implement the following interfaces
To mock listers for PipelineRuns.
1. [PipelineRunLister](./../pkg/client/listers/pipeline/v1alpha1/pipelinerun.go#L26)
1. [PipelineRunNamespaceLister](./../pkg/client/listers/pipeline/v1alpha1/pipelinerun.go#L58)

You can look at example mock for PipelineRunContollerTest [here](./../pkg/reconciler/v1alpha1/pipelinerun/pipelinerun_test.go#L123)
In this example, we have used the same struct which implements both the interfaces.

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
```