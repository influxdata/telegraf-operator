# Kustomization with telegraf-operator

This examples shows how to use the telegraf-operator to add monitoring to resources managed by Kustomize

We're taking a look at an example about deploying mysql

## Usage

1. Install `telegraf-operator`
2. Follow instructions [Here](https://github.com/kubernetes-sigs/kustomize/tree/master/examples/mySql)
3. Run `kustomize build` in this directory

## What's happening?

We added a patch to add the necessary annotations to enable the telegraf-operator

This is a non intrusive manner of adding monitoring to mysql without distrupting existing deployments

THis pattern can be applied to any sort of deployment with Kustomize
