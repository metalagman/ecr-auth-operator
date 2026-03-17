# Contributing

## Local Development

Prerequisites:

- Go 1.24+
- Docker
- kubectl
- Access to a Kubernetes cluster
- Helm

Run tests (unit + envtest):

```sh
task test
```

Build:

```sh
task build
```

Validate the Helm chart locally:

```sh
task helm-lint
task helm-template
```
