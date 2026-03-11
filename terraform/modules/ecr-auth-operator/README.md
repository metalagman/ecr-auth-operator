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
  source = "../../modules/ecr-auth-operator"

  basename            = "ecr-auth"
  # release_name defaults to "${basename}-operator"
  # operator_namespace defaults to "${basename}-operator-system"
  # credentials_secret_name defaults to "${basename}-aws-credentials"

  create_namespace    = true

  chart_version       = "0.0.1"
  image_repository    = "ghcr.io/metalagman/ecr-auth-operator"
  image_tag           = "0.0.1"

  # Keep true to let the module create IAM user + access keys.
  create_iam_user     = true
  # iam_user_name defaults to "${basename}-operator"
}
```

## Use existing AWS credentials instead of creating IAM user

```hcl
module "ecr_auth_operator" {
  source = "../../modules/ecr-auth-operator"

  create_iam_user       = false
  aws_access_key_id     = var.aws_access_key_id
  aws_secret_access_key = var.aws_secret_access_key
}
```

## Providers

Configure providers in the root module (AWS, Kubernetes, Helm). This module does not configure providers itself.
