# Contributing to InfluxData Documentation

## Sign the InfluxData CLA
The InfluxData Contributor License Agreement (CLA) is part of the legal framework
for the open-source ecosystem that protects both you and InfluxData.
To make substantial contributions to InfluxData telegraf-operator, first sign the InfluxData CLA.
What constitutes a "substantial" change is at the discretion of InfluxData documentation maintainers.

[Sign the InfluxData CLA](https://www.influxdata.com/legal/cla/)

_**Note:** Typo and broken link fixes are greatly appreciated and do not require signing the CLA._

## Contributing to telegraf-operator

We have put effort into making contributing to `telegraf-operator` as easy as possible.

### Fork and clone InfluxData Documentation Repository

[Fork this repository](https://help.github.com/articles/fork-a-repo/) and
[clone it](https://help.github.com/articles/cloning-a-repository/) to your local machine.

### Test driven develpoment

Due to complexity of how operators work, the recommended way to implement changes is to also implement testing to ensure your changes work as expected.

Tests should include both the happy path as well as all edge cases. This speeds up development compared to verifying code in Kubernetes-based environments.

In order to run tests, simply run the `test` target - such as:

```shell
make test
```

This runs linting, vetting as well as tests themselves. The output from the command also includes a report including test coverage.

## Testing your code in Kubernetes

### Setting up Kubernetes in Docker and local test cluster

The recommended way of testing changes made to telegraf-operator in real environments is to use [kind](https://github.com/kubernetes-sigs/kind).
This allows easy local development as long as [Docker](https://docs.docker.com/install/) is available locally on your machine.

The `kind` CLI is also needed and the
[kind Quick Start guide](https://kind.sigs.k8s.io/docs/user/quick-start/) describes the process of installing it.

The `Makefile` bundled with `telegraf-operator` includes several targets that simplify testing your changes in Kubernetes.

The `kind-start` target invokes `kind` CLI to create a new cluster called `telegraf-operator-test` that can be used to test `telegraf-operator`:

```shell
make kind-start
```

This operation deploys InfluxDB v1 along with Chronograf in the cluster, so that it is possible to verify the data is sent by Telegraf.
Deployment of InfluxDB is not needed to work or use `telegraf-operator`, but is included as a convenience.

### Building telegraf-operator image locally

In order to build a container image based on local clone of the `telegraf-repository`, simply run the `kind-build` command.

```shell
make kind-build
```

This will build the telegraf-operator with the same image tag as the official image and will inject the image into the `telegraf-operator-test` kind cluster.

### Deploying telegraf-operator and testing it

The `kind-test` target deploys telegraf operator - by deploying [examples/classes.yml](examples/classes.yml) and [deploy/dev.yml](deploy/dev.yml) files to your local kind cluster.

```shell
make kind-test
```

This will deploy `telegraf-operator` along with the mutating webhook configuration that will instruct kind cluster to allow `telegraf-operator` to add the `telegraf` sidecar container for any newly created pods.

It will also deploy a `redis` statefulset in `test` namespace that allows verifying that `redis` was deployed with the telegraf sidecar container added.

If the `kind-build` target was run prior to this command, the locally built image of `telegraf-operator` will be used. If not, the upstream image of `telegraf-operator` will be pulled by kind cluster.

### Restarting telegraf-operator

The `kind-delete-pod` target deletes the `telegraf-operator` pod(s), so that Kubernetes restarts them.

```shell
make kind-delete-pod
```
Re-creating the pod will cause kind cluster to pick up any newer image of `telegraf-operator` that was injected into the kind cluster.

This can be used along with `kind-build` target to first build a new image of telegraf-operator and then force the kind cluster to create a new pod.

### Deleting telegraf-operator, test namespace and other objects

The `kind-cleanup` target provides a way to delete all of the resources related to `telegraf-operator` from the kind cluster.

```shell
make kind-cleanup
```

This command will delete `telegraf-operator` and `test` namespaces as well as mutating webhook configuration.

It's useful when ensuring that next invocation of `kind-test` will re-create telegraf-operator from scratch, without having to delete and create the entire kind cluster.

Note that this command will keep the InfluxDB v1 instance running in your kind cluster.

### Deleting the test cluster

The target `kind-delete` can be used to delete the entire `telegraf-operator-test` kind cluster.

```shell
make kind-delete
```

This will delete the `telegraf-operator-test` kind cluster along with any resources that were deployed into it.

### Makefile targets

The list below contains a short version of `make` targets, based on description above:

* `kind-start` - start a new kind (Kubernetes in Docker) cluster for `telegraf-operator` testing, along with an InfluxDB instance
* `kind-build` - build an image of `telegraf-operator` locally and inject it into the kind cluster
* `kind-test` - deploy `telegraf-operator` along with any additional resources as well as a `redis` statefulset in `test` namespace
* `kind-delete-pod` - deletes `telegraf-operator` pod(s), which triggers a restart and new container image of `telegraf-operator` to be used, if it was injected
* `kind-cleanup` - deletes all resources related to `telegraf-operator` from the kind cluster, leaving InfluxDB running
* `kind-delete` - deletes the entire kind cluster
