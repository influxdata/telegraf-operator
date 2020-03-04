# Telegraf-operator

[![Docker Repository on Quay](https://quay.io/repository/influxdb/telegraf-operator/status "Docker Repository on Quay")](https://quay.io/repository/influxdb/telegraf-operator)
[![CircleCI](https://circleci.com/gh/influxdata/telegraf-operator/tree/master.svg?style=svg)](https://circleci.com/gh/influxdata/telegraf-operator/tree/master)

# The motto
Easy things should be easy. Adding monitoring to your application has never been as easy as now. 

Does your application exposes prometheus metrics? then adding `telegraf.influxdata.com/port: 8080` annotation to the pod is the only thing you need to add telegraf scraping to it

# Why telegraf-operator?

No one likes monitoring/observability, everybody wants to deploy applications but the burden of adding monitoring, fixing it, maintaining it should not weight that much. 

# Installation

We don't provide yet a production-like deployment of `telegraf-operator` given the alpha status of the project 

But we provide a development version that can be installed by running

```shell
kubectl apply -f https://github.com/influxdata/telegraf-operator/blob/master/deploy/dev.yml
```

# Usage

The available annotions are:
- `telegraf.influxdata.com/port`: is used to configure which port telegraf should scrape
- `telegraf.influxdata.com/ports` : is used to configure which port telegraf should scrape, comma separated list of ports to scrape
- `telegraf.influxdata.com/path` : is used to configure at which path to configure scraping to (a port must be configured also), will apply to all ports if multiple are configured
- `telegraf.influxdata.com/scheme` : is used to configure at the scheme for the metrics to scrape, will apply to all ports if multiple are configured ( only `http` or `https` are allowed as values)
- `telegraf.influxdata.com/interval` : is used to configure interval for telegraf scraping (Go style duration, e.g 5s, 30s, 2m .. )
- `telegraf.influxdata.com/inputs` : is used to configure custom inputs for telegraf
- `telegraf.influxdata.com/internal` : is used to enable telegraf "internal" input plugins for
- `telegraf.influxdata.com/class` : configures which kind of class to use (classes are configured on the operator)
- `telegraf.influxdata.com/secret-env` : allows adding secrets to the telegraf sidecar in the form of environment variables

### Maintainer

- [gitirabassi](https://github.com/gitirabassi)
- [rawkode](https://github.com/rawkode)
