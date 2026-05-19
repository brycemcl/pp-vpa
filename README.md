# pp-vpa

Per-Pod Vertical Pod Autoscaler — PSI-driven, edge-actuated in-place updates. See the [architecture doc](Per-Pod%20Vertical%20Pod%20Autoscaler%20%28PP-VPA%29%20Architecture.md) for the design.

## Install

### Helm (Pages repo)

```sh
helm repo add pp-vpa https://brycemclachlan.github.io/pp-vpa
helm repo update
helm install pp-vpa pp-vpa/pp-vpa \
  --namespace pp-vpa-system --create-namespace
```

### Helm (OCI registry)

```sh
helm install pp-vpa oci://ghcr.io/brycemclachlan/charts/pp-vpa \
  --version 0.1.0 \
  --namespace pp-vpa-system --create-namespace
```

### Container image directly

```sh
docker pull ghcr.io/brycemclachlan/pp-vpa:latest
```

Available tags:
- `latest` / `vX.Y.Z` — published from a release tag
- `main` — rolling build of the main branch
- `sha-<commit>` — pinnable per-commit tag

Images are multi-arch (`linux/amd64`, `linux/arm64`) and signed with cosign (keyless via GitHub Actions OIDC):

```sh
cosign verify ghcr.io/brycemclachlan/pp-vpa:v0.1.0 \
  --certificate-identity-regexp 'https://github.com/brycemclachlan/pp-vpa/.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Build locally

```sh
make docker-build docker-push IMG=<some-registry>/pp-vpa:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/pp-vpa:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/pp-vpa:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/pp-vpa/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

### Git hooks

The repo ships a pre-commit hook in `.githooks/` that rejects commits with unformatted Go files. After cloning:

```sh
git config core.hooksPath .githooks
```

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

