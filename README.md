# Telegraf-operator

[![Docker Repository on Quay](https://quay.io/repository/influxdb/telegraf-operator/status "Docker Repository on Quay")](https://quay.io/repository/influxdb/telegraf-operator)
[![CircleCI](https://circleci.com/gh/influxdata/telegraf-operator/tree/master.svg?style=svg)](https://circleci.com/gh/influxdata/telegraf-operator/tree/master)

# The motto
Easy things should be easy. Adding monitoring to your application has never been as easy as now.

Does your application exposes prometheus metrics? then adding `telegraf.influxdata.com/port: 8080` annotation to the pod is the only thing you need to add telegraf scraping to it

# Why telegraf-operator?

No one likes monitoring/observability, everybody wants to deploy applications but the burden of adding monitoring, fixing it, maintaining it should not weight that much.

# Getting started with telegraf-operator

Releasing docker images at: [Quay](https://quay.io/repository/influxdb/telegraf-operator?tag=latest&tab=tags)

## Adding telegraf-operator in development mode

We don't provide yet a production-like deployment of `telegraf-operator` given the alpha status of the project

But we provide a development version that can be installed by running

```shell
kubectl apply -f https://raw.githubusercontent.com/influxdata/telegraf-operator/master/deploy/dev.yml 
```

The command above deploys telegraf-operator, using a separate `telegraf-operator` namespace and registering webhooks that will inject a telegraf sidecar to all newly created pods.

In order to use `telegraf-operator`, what's also needed is to define where metrics should be sent.
The [examples/classes.yml](examples/classes.yml) file provides a set of classes that can be used to get started.

To create sample set of classes, simply run:

```shell
kubectl apply -f https://raw.githubusercontent.com/influxdata/telegraf-operator/master/examples/classes.yml
```

## Installing InfluxDB for data retrieval

In order to see the data, you can also deploy [InfluxDB](https://github.com/influxdata/influxdb/) v1 in your cluster, which also comes with [Chronograf](https://www.influxdata.com/time-series-platform/chronograf/), providing a web UI for InfluxDB v1.

To set it up in your cluster, simply run:

```shell
kubectl apply -f https://raw.githubusercontent.com/influxdata/telegraf-operator/master/deploy/influxdb.yml 
```

After that, every new pod (created directly or by creating a deployment or statefulset) in your cluster will have include telegraf container for retrieving data.

## Installing a sample application with telegraf-operator based monitoring set up

You can try it by running one of our samples - such as a redis server. Simply do:

```shell
kubectl apply -f https://raw.githubusercontent.com/influxdata/telegraf-operator/master/examples/redis.yml
```

You can verify the telegraf container is present by doing:

```shell
kubectl describe pod -n redis redis-0
```

The output should include a `telegraf` container.

In order to see the results in InfluxDB and Chronograf, you will need to set up port-forwarding and then access Chronograf from your browser:

```shell
kubectl port-forward --namespace=influxdb svc/influxdb 8888:8888
```

Next, go to [http://localhost:8888](http://localhost:8888) and continue to [Explore](http://localhost:8888/sources/0/chronograf/data-explorer) section to see your data 

# Configuration and usage

Telegraf-operator consists of the following:

* Global configuration - definition of where the metrics should be sent and other auxiliary configuration, specified as classes
* Pod-level configuration - definition of how a pod can be monitored, such as ports for Prometheus scraping and additional configurations

## Global configuration - classes

Telegraf-operator is based on concepts of globally defined classes. Each class is a subset of Telegraf configuration and usually defines where Telegraf should be sending its outputs, along with other settings such as global tags.

Usually classes are defined as a secret - such as in [classes.yml](examples/classes.yml) file - and each class maps to a key in a secret. For example:

```
stringData:
  basic: |+
    [[outputs.influxdb]]
      urls = ["http://influxdb.influxdb:8086"]
    [[outputs.file]]
      files = ["stdout"]
    [global_tags]
      hostname = "$HOSTNAME"
      nodename = "$NODENAME"
      type = "app"
```

The above defines that any pod whose Telegraf class is `basic` will have its metrics sent to a specific URL, which in this case is an InfluxDB v1 instance deployed in same cluster. Its metrics will also be logged by `telegraf` container for convenience. The data will also have `hostname`, `nodename` and `type` tags added for all metrics.

## Pod-level annotations

Each pod (either standalone or as part of deployment as well as statefulset) may also specify how it should be monitored using metadata.

The [redis.yml](examples/redis.yml) example adds annotation that enables the Redis plugin so that Telegraf will automatically retrieve metrics related to it.

```
apiVersion: apps/v1
kind: StatefulSet
  # ...
spec:
  template:
    metadata:
      annotations:
        telegraf.influxdata.com/inputs: |+
          [[inputs.redis]]
            servers = ["tcp://localhost:6379"]
        telegraf.influxdata.com/class: basic
      # ...
    spec:
      containers:
      - name: redis
        image: redis:alpine
```

Please see [redis input plugin documentation](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/redis) for more details on how the plugin can be configured.

The `telegraf.influxdata.com/class` specifies that the `basic` class above should be used.

Users can configure the `inputs.prometheus` plugin by setting the following annotations. Below is an [example configuration](#example-prometheus-scraping), and the expected output.
- `telegraf.influxdata.com/port`: is used to configure which port telegraf should scrape
- `telegraf.influxdata.com/ports` : is used to configure which port telegraf should scrape, comma separated list of ports to scrape
- `telegraf.influxdata.com/path` : is used to configure at which path to configure scraping to (a port must be configured also), will apply to all ports if multiple are configured
- `telegraf.influxdata.com/scheme` : is used to configure at the scheme for the metrics to scrape, will apply to all ports if multiple are configured ( only `http` or `https` are allowed as values)
- `telegraf.influxdata.com/interval` : is used to configure interval for telegraf scraping (Go style duration, e.g 5s, 30s, 2m .. )

### Example Prometheus Scraping

```
apiVersion: apps/v1
kind: StatefulSet
  # ...
spec:
  template:
    metadata:
      annotations:
        telegraf.influxdata.com/class: influxdb # User defined output class
        telegraf.influxdata.com/interval: 30s
        telegraf.influxdata.com/path: /metrics
        telegraf.influxdata.com/port: "8086"
        telegraf.influxdata.com/scheme: http
      # ...
    spec:
      containers:
      - name: influxdb
        image: quay.io/influxdb/influxdb:v2.0.4
```

#### Configuration Output

```
[[inputs.prometheus]]
  urls = ["http://127.0.0.1:8086/metrics"]
  interval = "30s"

[[inputs.internal]]
```


Additional pod annotations that can be used to configure the Telegraf sidecar:
- `telegraf.influxdata.com/inputs` : is used to configure custom inputs for telegraf
- `telegraf.influxdata.com/internal` : is used to enable telegraf "internal" input plugins for
- `telegraf.influxdata.com/image` : is used to configure telegraf image to be used for the `telegraf` sidecar container
- `telegraf.influxdata.com/class` : configures which kind of class to use (classes are configured on the operator)
- `telegraf.influxdata.com/secret-env` : allows adding secrets to the telegraf sidecar in the form of environment variables
- `telegraf.influxdata.com/requests-cpu` : allows specifying resource requests for CPU
- `telegraf.influxdata.com/requests-memory` : allows specifying resource requests for memory
- `telegraf.influxdata.com/limits-cpu` : allows specifying resource limits for CPU
- `telegraf.influxdata.com/limits-memory` : allows specifying resource limits for memory


# Contributing to telegraf-operator

Please read the [CONTRIBUTING](CONTRIBUTING.md) file for more details on how to get started with contributing to to `telegraf-operator`.

# Maintainers

- [gitirabassi](https://github.com/gitirabassi)
- [rawkode](https://github.com/rawkode)
- [wojciechka](https://github.com/wojciechka/)
