output "helm_release_name" {
  description = "Installed Helm release name."
  value       = helm_release.operator.name
}

output "operator_namespace" {
  description = "Namespace where the operator is installed."
  value       = helm_release.operator.namespace
}

output "aws_credentials_secret_name" {
  description = "Kubernetes secret name used by the operator for AWS credentials."
  value       = kubernetes_secret_v1.aws_credentials.metadata[0].name
}

output "aws_credentials_secret_namespace" {
  description = "Namespace for the Kubernetes AWS credentials secret."
  value       = kubernetes_secret_v1.aws_credentials.metadata[0].namespace
}

output "iam_user_name" {
  description = "Created IAM user name, or null when create_iam_user is false."
  value       = var.create_iam_user ? aws_iam_user.operator[0].name : null
}

output "iam_access_key_id" {
  description = "IAM access key id used by the operator."
  value       = local.effective_access_key_id
  sensitive   = true
}

output "generated_aws_secret_access_key" {
  description = "Generated IAM secret access key when create_iam_user is true."
  value       = var.create_iam_user ? aws_iam_access_key.operator[0].secret : null
  sensitive   = true
}
