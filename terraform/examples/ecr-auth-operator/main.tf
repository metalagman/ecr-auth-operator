terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 3.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kubeconfig_context != "" ? var.kubeconfig_context : null
}

provider "helm" {
  kubernetes = {
    config_path    = var.kubeconfig_path
    config_context = var.kubeconfig_context != "" ? var.kubeconfig_context : null
  }
}

module "ecr_auth_operator" {
  source = "../../modules/ecr-auth-operator"

  create_iam_user       = var.create_iam_user
  iam_user_name         = var.iam_user_name
  aws_access_key_id     = var.aws_access_key_id
  aws_secret_access_key = var.aws_secret_access_key
  aws_session_token     = var.aws_session_token

  operator_namespace           = var.operator_namespace
  create_namespace             = var.create_namespace
  credentials_secret_name      = var.credentials_secret_name
  credentials_secret_namespace = var.credentials_secret_namespace

  release_name     = var.release_name
  chart_repository = var.chart_repository
  chart_name       = var.chart_name
  chart_version    = var.chart_version

  image_repository = var.image_repository
  image_tag        = var.image_tag
}

output "release_name" {
  value = module.ecr_auth_operator.helm_release_name
}

output "credentials_secret_ref" {
  value = {
    name      = module.ecr_auth_operator.aws_credentials_secret_name
    namespace = module.ecr_auth_operator.aws_credentials_secret_namespace
  }
}

output "iam_access_key_id" {
  value     = module.ecr_auth_operator.iam_access_key_id
  sensitive = true
}
