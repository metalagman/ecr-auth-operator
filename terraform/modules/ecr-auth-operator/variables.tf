variable "basename" {
  description = "Base name prefix for AWS IAM resources created by this module (for example: ecr-auth)."
  type        = string
  default     = "ecr-auth"

  validation {
    condition     = length(trimspace(var.basename)) > 0
    error_message = "basename must not be empty."
  }
}

variable "create_iam_user" {
  description = "Create a dedicated IAM user and access key for the operator."
  type        = bool
  default     = true
}

variable "iam_user_name" {
  description = "IAM user name to create when create_iam_user is true. Null uses <basename>-operator."
  type        = string
  default     = null
}

variable "iam_user_path" {
  description = "IAM path for the created user."
  type        = string
  default     = "/"
}

variable "iam_user_force_destroy" {
  description = "Delete IAM access keys and other attached resources with the IAM user during destroy."
  type        = bool
  default     = false
}

variable "aws_access_key_id" {
  description = "Existing AWS access key id to use when create_iam_user is false."
  type        = string
  default     = ""
  sensitive   = true

  validation {
    condition     = var.create_iam_user || length(trimspace(var.aws_access_key_id)) > 0
    error_message = "aws_access_key_id must be provided when create_iam_user is false."
  }
}

variable "aws_secret_access_key" {
  description = "Existing AWS secret access key to use when create_iam_user is false."
  type        = string
  default     = ""
  sensitive   = true

  validation {
    condition     = var.create_iam_user || length(trimspace(var.aws_secret_access_key)) > 0
    error_message = "aws_secret_access_key must be provided when create_iam_user is false."
  }
}

variable "aws_session_token" {
  description = "Optional session token stored in the Kubernetes credentials secret."
  type        = string
  default     = ""
  sensitive   = true
}

variable "operator_namespace" {
  description = "Namespace where the operator Helm release is installed."
  type        = string
  default     = "ecr-auth-operator-system"
}

variable "create_namespace" {
  description = "Create operator_namespace before secret/Helm installation."
  type        = bool
  default     = true
}

variable "credentials_secret_name" {
  description = "Name of the Kubernetes Secret containing AWS credentials for the operator."
  type        = string
  default     = "aws-credentials"
}

variable "credentials_secret_namespace" {
  description = "Namespace for the credentials secret. Defaults to operator_namespace when null."
  type        = string
  default     = null
}

variable "release_name" {
  description = "Helm release name."
  type        = string
  default     = "ecr-auth-operator"
}

variable "chart_repository" {
  description = "Helm OCI/chart repository URL."
  type        = string
  default     = "oci://ghcr.io/metalagman/charts"
}

variable "chart_name" {
  description = "Chart name within the repository."
  type        = string
  default     = "ecr-auth-operator"
}

variable "chart_version" {
  description = "Chart version to install. Null uses repository default."
  type        = string
  default     = null
}

variable "image_repository" {
  description = "Controller image repository passed to chart values."
  type        = string
  default     = "ghcr.io/metalagman/ecr-auth-operator"
}

variable "image_tag" {
  description = "Controller image tag passed to chart values."
  type        = string
  default     = "latest"
}

variable "helm_atomic" {
  description = "Run Helm install/upgrade atomically."
  type        = bool
  default     = true
}
