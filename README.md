# ecr-auth-operator

`ecr-auth-operator` is a Kubernetes operator that manages Docker pull secrets (`kubernetes.io/dockerconfigjson`) from AWS ECR authorization tokens.

It watches namespaced `ECRAuth` custom resources (`ecr.metalagman.dev/v1alpha1`) and creates/refreshes the target secret in the same namespace.

## What It Does

- Watches `ECRAuth` resources across all namespaces.
- Retrieves ECR auth tokens using controller pod IAM credentials.
- Optionally assumes `spec.roleArn` before token retrieval.
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
- `region` (string, required): AWS region for ECR token retrieval.
- `refreshInterval` (duration, optional, default `11h`): refresh cadence.
- `roleArn` (string, optional): IAM role to assume via STS before ECR call.

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
  region: us-east-1
  refreshInterval: 11h
  # roleArn: arn:aws:iam::123456789012:role/ecr-reader
```

## Local Development

Prerequisites:

- Go 1.24+
- Docker
- kubectl
- Access to a Kubernetes cluster
- Helm (for chart validation/install)

Run tests (unit + envtest):

```sh
make test
```

Build:

```sh
make build
```

## Deploy with Kustomize Manifests

Install CRD:

```sh
make install
```

Deploy controller:

```sh
make deploy IMG=<registry>/ecr-auth-operator:<tag>
```

Apply sample CR:

```sh
kubectl apply -f config/samples/ecr_v1alpha1_ecrauth.yaml
```

## Deploy with Helm

Chart path:

- `charts/ecr-auth-operator`

Validate chart:

```sh
make helm-lint
make helm-template
```

Install:

```sh
helm upgrade --install ecr-auth-operator charts/ecr-auth-operator \
  --namespace ecr-auth-operator-system \
  --create-namespace \
  --set image.repository=<registry>/ecr-auth-operator \
  --set image.tag=<tag>
```

## Operational Notes

- Managed secrets are labeled `ecr.metalagman.dev/managed-by=ecr-auth-operator`.
- On CR deletion, managed secret cleanup relies on owner references (garbage collection).
- If `secretName` changes, the previous managed secret owned by the same CR is cleaned up by reconcile.

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0.
