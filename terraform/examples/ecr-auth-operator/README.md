# Example: install ecr-auth-operator with Terraform

## 1. Create `terraform.tfvars`

```hcl
aws_region        = "us-east-1"
kubeconfig_path   = "~/.kube/config"
kubeconfig_context = ""

# By default this example creates IAM user + access key for the operator.
create_iam_user = true

# Optionally pin release artifacts.
chart_version = "0.0.1"
image_tag     = "0.0.1"
```

## 2. Apply

```bash
terraform init
terraform apply
```

## 3. Use module outputs

The example exports:

- `release_name`
- `credentials_secret_ref`
- `iam_access_key_id` (sensitive)
