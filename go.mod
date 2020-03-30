module github.com/influxdata/telegraf-operator

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/influxdata/toml v0.0.0-20180607005434-2a2e3012f7cf
	github.com/naoina/go-stringutil v0.1.0 // indirect
	k8s.io/api v0.0.0-20190918155943-95b840bb6a1f
	k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/apiserver v0.0.0-20190918160949-bfa5e2e684ad
	k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
	sigs.k8s.io/controller-runtime v0.4.0
)
