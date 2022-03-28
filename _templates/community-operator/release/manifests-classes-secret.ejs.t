---
to: <%= output %>/<%= version %>/manifests/classes-secret.yaml
---
# initial configuration of telegraf-operator's classes
apiVersion: v1
kind: Secret
metadata:
  name: classes
stringData:
  default: |
    [global_tags]
      type = "default"
    [[outputs.file]]
      files = ["stdout"]
