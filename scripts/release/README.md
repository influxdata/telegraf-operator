Helper tools for releasing a new version of telegraf-operator.

The `community-operators` logic depends on [Hygen](https://www.hygen.io/) and was tested to work with `v6.1.0`.

## Usage

### Manually release a new version in Github

In the github UI create a new [release](https://github.com/influxdata/telegraf-operator/releases) selecting a tag with the new version number. 
The release should be created from the `master` branch. List the new features and bug fixes in the release description and 
create the release.

### Update the community-operators hub and Helm chart repos

Clone `influxdata/helm-chart`s and fork and clone the `k8s-operatorhub/community-operators` repo into the same directory as telegraf-operator (i.e. `telegraf-operator`, `helm-charts` and forked `community-operators` should have same parent directory).

then run the following command:

```bash
./scripts/release/update.bash $VERSION
```
where `$VERSION` is the version of telegraf-operator to release that you created earlier in Github. Do not include the `v` prefix for the version.
Example: `./scripts/release/update.bash 0.1.0`

This create commits in the `community-operators` and `helm-chart` repos with the new version of telegraf-operator.
Push the changes to your fork and open a PR to merge the changes into the `k8s-operatorhub/community-operators` repo and
push the changes to the `influxdata/helm-chart` repo and create a PR. Once the PRs are merged, the new version of telegraf-operator
will be available in the OperatorHub and the Helm repo.