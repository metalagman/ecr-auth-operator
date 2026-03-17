variable "aws_region" {
  description = "AWS region for IAM API calls."
  type        = string
}

variable "kubeconfig_path" {
  description = "Path to kubeconfig used by Kubernetes and Helm providers."
  type        = string
  default     = "~/.kube/config"
}

variable "kubeconfig_context" {
  description = "Optional kubeconfig context."
  type        = string
  default     = ""
}

variable "create_iam_user" {
  description = "Create IAM user and access key for the operator."
  type        = bool
  default     = true
}

variable "iam_user_name" {
  description = "IAM user name when create_iam_user is true."
  type        = string
  default     = "ecr-auth-operator"
}

variable "aws_access_key_id" {
  description = "Existing AWS access key id when create_iam_user is false."
  type        = string
  default     = ""
  sensitive   = true
}

variable "aws_secret_access_key" {
  description = "Existing AWS secret access key when create_iam_user is false."
  type        = string
  default     = ""
  sensitive   = true
}

variable "aws_session_token" {
  description = "Optional AWS session token."
  type        = string
  default     = ""
  sensitive   = true
}

variable "operator_namespace" {
  description = "Namespace where the operator is installed."
  type        = string
  default     = "ecr-auth-operator-system"
}

variable "create_namespace" {
  type    = bool
  default = true
}

variable "credentials_secret_name" {
  description = "Kubernetes secret name for AWS credentials."
  type        = string
  default     = "aws-credentials"
}

variable "credentials_secret_namespace" {
  type    = string
  default = null
}

variable "release_name" {
  description = "Helm release name."
  type        = string
  default     = "ecr-auth-operator"
}

variable "chart_repository" {
  type    = string
  default = "oci://ghcr.io/metalagman/charts"
}

variable "chart_name" {
  type    = string
  default = "ecr-auth-operator"
}

variable "chart_version" {
  type    = string
  default = "0.0.5"
}

variable "image_repository" {
  type    = string
  default = "ghcr.io/metalagman/ecr-auth-operator"
}

variable "image_tag" {
  type    = string
  default = "0.0.5"
}
