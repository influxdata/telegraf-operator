module github.com/influxdata/telegraf-operator

go 1.16

require (
	github.com/go-logr/logr v0.3.0
	github.com/influxdata/toml v0.0.0-20180607005434-2a2e3012f7cf
	github.com/naoina/go-stringutil v0.1.0 // indirect
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/apiserver v0.20.1
	k8s.io/client-go v0.20.2
	sigs.k8s.io/controller-runtime v0.8.3
)
