# Helm with telegraf-operator

This examples shows how to use the telegraf-operator to add monitoring to resources managed by Helm

## Usage

1. Install `telegraf-operator`
2. install the Helm repo

```shell
helm repo add elastic https://helm.elastic.co
```

3. install with new annotations

we just need to substitute
```yaml
podAnnotations: {}
```

with 

```yaml
podAnnotations:
  telegraf.influxdata.com/inputs: |+
    [[inputs.elasticsearch]]
      servers = ["http://localhost:9200"]
  telegraf.influxdata.com/class: infra
```

4. Install elasticsearch with newly changed values file

```shell
helm install --values values.yml --name elasticsearch elastic/elasticsearch
```

## Reasoning

Most helm charts expose the possibility to add custom annotations to pods enabling us to use telegraf-operator with exisiting deployment without disrupting existing practices

### References
https://github.com/elastic/helm-charts/tree/master/elasticsearch
