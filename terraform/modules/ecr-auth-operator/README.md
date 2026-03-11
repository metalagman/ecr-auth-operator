# ecr-auth-operator Terraform module

This module installs `ecr-auth-operator` with Helm and prepares AWS credentials in Kubernetes.

## What it creates

- Optional IAM user + access key for the operator (`create_iam_user = true`)
- IAM inline policy granting `ecr:GetAuthorizationToken`
- Kubernetes Secret with keys:
  - `aws_access_key_id`
  - `aws_secret_access_key`
  - `aws_session_token` (optional)
- Helm release of `ecr-auth-operator` wired to that credentials secret

## Usage

```hcl
module "ecr_auth_operator" {
  source = "github.com/metalagman/ecr-auth-operator//terraform/modules/ecr-auth-operator?ref=v0.0.2"

  release_name        = "ecr-auth-operator"
  operator_namespace  = "ecr-auth-operator-system"
  credentials_secret_name = "aws-credentials"
  create_namespace    = true

  chart_version       = "0.0.2"
  image_repository    = "ghcr.io/metalagman/ecr-auth-operator"
  image_tag           = "0.0.2"

  # Keep true to let the module create IAM user + access keys.
  create_iam_user     = true
  iam_user_name       = "ecr-auth-operator"
}
```

## Use existing AWS credentials instead of creating IAM user

```hcl
module "ecr_auth_operator" {
  source = "github.com/metalagman/ecr-auth-operator//terraform/modules/ecr-auth-operator?ref=v0.0.2"

  create_iam_user       = false
  aws_access_key_id     = var.aws_access_key_id
  aws_secret_access_key = var.aws_secret_access_key
}
```

## Providers

Configure providers in the root module (AWS, Kubernetes, Helm). This module does not configure providers itself.
