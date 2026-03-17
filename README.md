# ecr-auth-operator

`ecr-auth-operator` is a Kubernetes operator that manages Docker pull secrets (`kubernetes.io/dockerconfigjson`) from AWS ECR authorization tokens.

It watches namespaced `ECRAuth` custom resources (`ecr.metalagman.dev/v1alpha1`) and creates/refreshes the target secret in the same namespace.

## What It Does

- Watches `ECRAuth` resources across all namespaces.
- Retrieves ECR auth tokens using static AWS credentials from a controller-global Kubernetes Secret.
- Derives AWS region from each registry endpoint in `spec.registries`.
- Creates or updates a managed pull secret (`spec.secretName`).
- Rejects unsafe cases:
  - Existing foreign secret with same name is not overwritten.
  - Second `ECRAuth` in same namespace targeting the same secret is rejected.
- Sets status conditions and last successful refresh timestamp.
- Requeues periodically (`spec.refreshInterval`, default `11h`).

## API

### Group Version Kind

- Group: `ecr.metalagman.dev`
- Version: `v1alpha1`
- Kind: `ECRAuth`
- Plural: `ecrauths`
- Scope: Namespaced

### Spec

- `secretName` (string, required): managed target secret name.
- `registries` (list, required): private ECR registry endpoints to include in the managed Docker config secret.
  Example item: `123456789012.dkr.ecr.us-east-1.amazonaws.com`
- `refreshInterval` (duration, optional, default `11h`): refresh cadence.

### Status

- `conditions[]` (`Ready` condition used).
- `observedGeneration`.
- `managedSecretName`.
- `lastSuccessfulRefreshTime`.

## Example

```yaml
apiVersion: ecr.metalagman.dev/v1alpha1
kind: ECRAuth
metadata:
  name: app-regcred
  namespace: app
spec:
  secretName: regcred
  registries:
    - 123456789012.dkr.ecr.us-east-1.amazonaws.com
    - 210987654321.dkr.ecr.eu-west-1.amazonaws.com
  refreshInterval: 11h
```

## AWS Credentials Secret

The controller requires a global Kubernetes Secret with the following keys:

- `aws_access_key_id` (required)
- `aws_secret_access_key` (required)
- `aws_session_token` (optional)

Example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
  namespace: ecr-auth-operator-system
type: Opaque
stringData:
  aws_access_key_id: AKIA...
  aws_secret_access_key: ...
```

Configure the controller with:

- `--aws-credentials-secret-name`
- `--aws-credentials-secret-namespace`

## Local Development

Prerequisites:

- Go 1.24+
- Docker
- kubectl
- Access to a Kubernetes cluster
- Helm (for chart validation/install)

Run tests (unit + envtest):

```sh
task test
```

Build:

```sh
task build
```

## Installation and Deployment

### Deploy with Terraform (recommended)

This repo includes a Terraform module that installs the Helm release and, by default, provisions:

- IAM user for the operator
- IAM inline policy for ECR auth/pull actions
- IAM access key for that user
- Kubernetes AWS credentials secret used by the controller

- Module: `terraform/modules/ecr-auth-operator`
- Example: `terraform/examples/ecr-auth-operator`
- GitHub module source: `github.com/metalagman/ecr-auth-operator//terraform/modules/ecr-auth-operator?ref=v0.0.2`

Set `create_iam_user = false` if you want to use existing AWS credentials instead of creating IAM resources.

Quick start:

```sh
cd terraform/examples/ecr-auth-operator
terraform init
terraform apply
```

### Deploy with Helm (OCI from GHCR)

OCI chart:

- `oci://ghcr.io/metalagman/charts/ecr-auth-operator`

Install:

```sh
helm upgrade --install ecr-auth-operator oci://ghcr.io/metalagman/charts/ecr-auth-operator \
  --version <chart-version> \
  --namespace ecr-auth-operator-system \
  --create-namespace \
  --set image.repository=ghcr.io/metalagman/ecr-auth-operator \
  --set image.tag=<image-tag> \
  --set awsCredentials.secretName=aws-credentials \
  --set awsCredentials.secretNamespace=ecr-auth-operator-system
```

Local chart validation during development:

```sh
task helm-lint
task helm-template
```

### Deploy with Kustomize manifests

Install CRD:

```sh
task manifests
kubectl apply -f config/crd/bases/ecr.metalagman.dev_ecrauths.yaml
```

Deploy controller:

```sh
kubectl apply -k config/default
```

Apply sample CR:

```sh
kubectl apply -f config/samples/ecr_v1alpha1_ecrauth.yaml
```

## OCI Release Publishing

GitHub Actions publishes OCI artifacts on semver tags (`vX.Y.Z`) using Taskfile release targets.

- Controller image: `ghcr.io/metalagman/ecr-auth-operator:<version>` and `:latest`
- Helm chart: `oci://ghcr.io/metalagman/charts/ecr-auth-operator:<version>`

The release workflow is defined in:

- `.github/workflows/release.yml`

## Operational Notes

- Managed secrets are labeled `ecr.metalagman.dev/managed-by=ecr-auth-operator`.
- On CR deletion, managed secret cleanup relies on owner references (garbage collection).
- If `secretName` changes, the previous managed secret owned by the same CR is cleaned up by reconcile.

## License

Copyright 2026 Alexey Samoylov.

Licensed under the Apache License, Version 2.0.
