---
to: <%= output %>/<%= version %>/manifests/telegraf-operator-v<%= version %>.clusterserviceversion.yaml
---
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: telegraf-operator.v<%= version %>
  annotations:
    containerImage: quay.io/influxdb/telegraf-operator:v<%= version %>
    imagePullPolicy: Always
    createdAt: <%= createdAt %>
    repository: https://github.com/influxdata/telegraf-operator
    support: https://github.com/influxdata/telegraf-operator
    description: >
      Telegraf Operator is a solution designed to create and manage individual Telegraf instances in Kubernetes clusters.
    capabilities: Seamless Upgrades
    categories: Monitoring
spec:
  description: >
    Telegraf Operator is a solution designed to create and manage individual Telegraf instances in Kubernetes clusters.


    Essentially, it functions as a control plane for managing the individual Telegraf instances deployed throughout your Kubernetes cluster.


    The operator will inject telegraf sidecar container to all newly created pods that have the appropriate [pod-level annotations](https://github.com/influxdata/telegraf-operator#pod-level-annotations).


    Telegraf-operator abstracts away how data gets stored and where it should be sent using classes, where each pod that will have its telegraf sidecar atted will also specify its application class, or it will use the `default` class if not provided.


    After deploying telegraf-operator you should modify the `classes` secret in appropriate namespace to configure telegraf-operator classes referenced via annotations. Each application class should provide output or outputs where the data should be sent, optionally also adding transformations that Telegraf supports.


    For more details on how to configure telegraf outputs, inputs and other sections, please refer to [Telegraf Documentation](https://docs.influxdata.com/telegraf/).
  version: <%= version %>
  maintainers:
    - email: wkocjan@influxdata.com
      name: Wojciech Kocjan
  maturity: stable
  icon:
    - base64data: PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0idXRmLTgiPz4KPCEtLSBHZW5lcmF0b3I6IEFkb2JlIElsbHVzdHJhdG9yIDIxLjEuMCwgU1ZHIEV4cG9ydCBQbHVnLUluIC4gU1ZHIFZlcnNpb246IDYuMDAgQnVpbGQgMCkgIC0tPgo8c3ZnIHZlcnNpb249IjEuMSIgaWQ9IkluZmx1eERhdGFfU3ltYm9sX09ubHkiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyIgeG1sbnM6eGxpbms9Imh0dHA6Ly93d3cudzMub3JnLzE5OTkveGxpbmsiCgkgeD0iMHB4IiB5PSIwcHgiIHdpZHRoPSI5MDBweCIgaGVpZ2h0PSI5MDBweCIgdmlld0JveD0iLTE3MyAtMTQzIDkwMCA5MDAiIHN0eWxlPSJlbmFibGUtYmFja2dyb3VuZDpuZXcgLTE3MyAtMTQzIDkwMCA5MDA7IgoJIHhtbDpzcGFjZT0icHJlc2VydmUiPgo8c3R5bGUgdHlwZT0idGV4dC9jc3MiPgoJLnN0MHtmaWxsOm5vbmU7fQoJLnN0MXtmaWxsOiMyOTI5MzM7fQo8L3N0eWxlPgo8cmVjdCBpZD0iQmFja2dyb3VuZCIgeD0iLTE3MyIgeT0iLTE0MyIgY2xhc3M9InN0MCIgd2lkdGg9IjkwMCIgaGVpZ2h0PSI5MDAiLz4KPHBhdGggaWQ9IkN1Ym9jdGFoZWRyb24iIGNsYXNzPSJzdDEiIGQ9Ik02OTQuMSwzOTQuOWwtODEtMzUyLjdDNjA4LjUsMjIuOSw1OTEsMy42LDU3MS43LTJMMjAxLjUtMTE2LjJjLTQuNi0xLjgtMTAuMS0xLjgtMTUuNy0xLjgKCWMtMTUuNywwLTMyLjIsNi40LTQzLjMsMTUuN2wtMjY1LjIsMjQ2LjhjLTE0LjcsMTIuOS0yMi4xLDM4LjctMTcuNSw1Ny4xbDg2LjYsMzc3LjZjNC42LDE5LjMsMjIuMSwzOC43LDQxLjQsNDQuMmwzNDYuMiwxMDYuOAoJYzQuNiwxLjgsMTAuMSwxLjgsMTUuNywxLjhjMTUuNywwLDMyLjItNi40LDQzLjMtMTUuN0w2NzYuNiw0NTNDNjkxLjQsNDM5LjIsNjk4LjcsNDE0LjMsNjk0LjEsMzk0Ljl6IE0yNDAuMi0zMi40bDI1NC4xLDc4LjMKCWMxMC4xLDIuOCwxMC4xLDcuNCwwLDEwLjFMMzYwLjgsODYuNGMtMTAuMSwyLjgtMjMuOS0xLjgtMzEuMy05LjJsLTkzLTEwMC40QzIyOC4yLTMxLjQsMjMwLTM1LjEsMjQwLjItMzIuNHogTTM5OC41LDQyMy41CgljMi44LDEwLjEtMy43LDE1LjctMTMuOCwxMi45bC0yNzQuNC04NC43Yy0xMC4xLTIuOC0xMi0xMS4xLTQuNi0xOC40TDMxNS43LDEzOGM3LjQtNy40LDE1LjctNC42LDE4LjQsNS41TDM5OC41LDQyMy41egoJIE0tNTMuNiwxNzQuOEwxNjkuMy0zMi40YzcuNC03LjQsMTkuMy02LjQsMjYuNywwLjlMMzA3LjQsODkuMmM3LjQsNy40LDYuNCwxOS4zLTAuOSwyNi43TDgzLjYsMzIzLjFjLTcuNCw3LjQtMTkuMyw2LjQtMjYuNy0wLjkKCUwtNTQuNSwyMDEuNkMtNjEuOSwxOTMuMy02MC45LDE4MS4zLTUzLjYsMTc0Ljh6IE0wLjgsNTAzLjZsLTU4LjktMjU4LjhjLTIuOC0xMC4xLDEuOC0xMiw4LjMtNC42bDkzLDEwMC40CgljNy40LDcuNCwxMC4xLDIyLjEsNy40LDMyLjJMMTAsNTAzLjZDNy4yLDUxMy43LDIuNiw1MTMuNywwLjgsNTAzLjZ6IE0zMjYuNyw2NTQuNmwtMjkxLTg5LjNjLTEwLjEtMi44LTE1LjctMTMuOC0xMi45LTIzLjkKCWw0OC44LTE1Ni42YzIuOC0xMC4xLDEzLjgtMTUuNywyMy45LTEyLjlsMjkxLDg5LjNjMTAuMSwyLjgsMTUuNywxMy44LDEyLjksMjMuOWwtNDguOCwxNTYuNkMzNDcsNjUxLjksMzM2LjksNjU3LjQsMzI2LjcsNjU0LjZ6CgkgTTU4NC41LDQ0Mi44TDM5MC4zLDYyMy4zYy03LjQsNy40LTExLDQuNi04LjMtNS41TDQyMi41LDQ4N2MyLjgtMTAuMSwxMy44LTIwLjMsMjMuOS0yMi4xbDEzMy41LTMwLjQKCUM1OTAuMSw0MzEuOCw1OTEuOSw0MzYuNCw1ODQuNSw0NDIuOHogTTYwNS43LDQwNC4yTDQ0NS41LDQ0MWMtMTAuMSwyLjgtMjAuMy0zLjctMjMtMTMuOGwtNjguMS0yOTYuNWMtMi44LTEwLjEsMy43LTIwLjMsMTMuOC0yMwoJbDE2MC4yLTM2LjhjMTAuMS0yLjgsMjAuMywzLjcsMjMsMTMuOGw2OC4xLDI5Ni41QzYyMi4zLDM5Mi4yLDYxNS45LDQwMi4zLDYwNS43LDQwNC4yeiIvPgo8L3N2Zz4K
      mediatype: image/svg+xml
  links:
    - name: GitHub
      url: https://github.com/influxdata/telegraf-operator
    - name: Issues
      url: https://github.com/influxdata/telegraf-operator/issues
    - name: Telegraf Documentation
      url: https://docs.influxdata.com/telegraf/
    - name: InfluxData Website
      url: https://www.influxdata.com
  displayName: Telegraf Operator
  provider:
    name: InfluxData
  install:
    spec:
      clusterPermissions:
        - rules:
          - apiGroups:
            - ""
            resources:
            - secrets
            verbs:
            - '*'
          - apiGroups:
            - ""
            resources:
            - namespaces
            verbs:
            - get
            - list
          - apiGroups:
            - ""
            resources:
            - pods
            verbs:
            - get
          serviceAccountName: telegraf-operator
      deployments:
        - name: telegraf-operator
          spec:
            replicas: 1
            selector:
              matchLabels:
                app.kubernetes.io/component: controller
                app.kubernetes.io/instance: telegraf-operator
                app.kubernetes.io/name: telegraf-operator
            template:
              metadata:
                labels:
                  app.kubernetes.io/component: controller
                  app.kubernetes.io/instance: telegraf-operator
                  app.kubernetes.io/name: telegraf-operator
                  telegraf.influxdata.com/ignore: "true"
              spec:
                serviceAccountName: telegraf-operator
                containers:
                  - name: telegraf-operator
                    image: quay.io/influxdb/telegraf-operator:v<%= version %>
                    imagePullPolicy: Always
                    args:
                      - "--cert-dir=/tmp/k8s-webhook-server/serving-certs"
                      - "--telegraf-default-class=default"
                      - "--telegraf-classes-directory=/etc/telegraf-operator"
                      - "--enable-default-internal-plugin"
                      - "--require-annotations-for-secret"
                      - "--telegraf-watch-config=inotify"
                    ports:
                      - name: https
                        containerPort: 9443
                        protocol: TCP
                    volumeMounts:
                      # TODO: certs
                      - mountPath: /etc/telegraf-operator
                        name: classes
                        readOnly: true
                    resources:
                      limits:
                        cpu: 1.0
                        memory: 256Mi
                      requests:
                        cpu: 100m
                        memory: 64Mi
                volumes:
                  - name: classes
                    secret:
                      secretName: "classes"
    strategy: deployment
  webhookdefinitions:
    - type: MutatingAdmissionWebhook
      admissionReviewVersions:
      - v1
      containerPort: 443
      targetPort: 9443
      deploymentName: telegraf-operator
      failurePolicy: Fail
      generateName: telegraf-operator.influxdata.com
      objectSelector:
        matchExpressions:
          - key: "telegraf.influxdata.com/ignore"
            operator: DoesNotExist
      rules:
        - apiGroups:
          - ''
          apiVersions:
          - 'v1'
          operations:
          - CREATE
          - DELETE
          resources:
          - pods
      sideEffects: None
      webhookPath: /mutate-v1-pod
  installModes:
    - supported: false
      type: OwnNamespace
    - supported: false
      type: SingleNamespace
    - supported: false
      type: MultiNamespace
    - supported: true
      type: AllNamespaces
  keywords:
    - telegraf
    - monitoring
    - metrics
    - scraping
