locals {
  credentials_secret_namespace = coalesce(var.credentials_secret_namespace, var.operator_namespace)

  operator_namespace_effective = var.create_namespace ? kubernetes_namespace_v1.operator[0].metadata[0].name : var.operator_namespace

  credentials_secret_namespace_effective = (
    var.create_namespace && local.credentials_secret_namespace == var.operator_namespace
  ) ? kubernetes_namespace_v1.operator[0].metadata[0].name : local.credentials_secret_namespace

  effective_access_key_id     = var.create_iam_user ? aws_iam_access_key.operator[0].id : var.aws_access_key_id
  effective_secret_access_key = var.create_iam_user ? aws_iam_access_key.operator[0].secret : var.aws_secret_access_key
}

resource "aws_iam_user" "operator" {
  count = var.create_iam_user ? 1 : 0

  name          = var.iam_user_name
  path          = var.iam_user_path
  force_destroy = var.iam_user_force_destroy
}

data "aws_iam_policy_document" "operator_ecr_auth" {
  count = var.create_iam_user ? 1 : 0

  statement {
    sid     = "AllowECRAuthorizationToken"
    effect  = "Allow"
    actions = ["ecr:GetAuthorizationToken"]
    resources = [
      "*"
    ]
  }
}

resource "aws_iam_user_policy" "operator_ecr_auth" {
  count = var.create_iam_user ? 1 : 0

  name   = "${var.iam_user_name}-ecr-auth"
  user   = aws_iam_user.operator[0].name
  policy = data.aws_iam_policy_document.operator_ecr_auth[0].json
}

resource "aws_iam_access_key" "operator" {
  count = var.create_iam_user ? 1 : 0

  user = aws_iam_user.operator[0].name
}

resource "kubernetes_namespace_v1" "operator" {
  count = var.create_namespace ? 1 : 0

  metadata {
    name = var.operator_namespace
  }
}

resource "kubernetes_secret_v1" "aws_credentials" {
  metadata {
    name      = var.credentials_secret_name
    namespace = local.credentials_secret_namespace_effective
  }

  type = "Opaque"

  data = merge(
    {
      aws_access_key_id     = local.effective_access_key_id
      aws_secret_access_key = local.effective_secret_access_key
    },
    length(trimspace(var.aws_session_token)) > 0 ? {
      aws_session_token = var.aws_session_token
    } : {}
  )
}

resource "helm_release" "operator" {
  name             = var.release_name
  repository       = var.chart_repository
  chart            = var.chart_name
  version          = var.chart_version
  namespace        = local.operator_namespace_effective
  create_namespace = false
  atomic           = var.helm_atomic
  cleanup_on_fail  = true

  set = [
    {
      name  = "image.repository"
      value = var.image_repository
    },
    {
      name  = "image.tag"
      value = var.image_tag
    },
    {
      name  = "awsCredentials.secretName"
      value = kubernetes_secret_v1.aws_credentials.metadata[0].name
    },
    {
      name  = "awsCredentials.secretNamespace"
      value = kubernetes_secret_v1.aws_credentials.metadata[0].namespace
    },
  ]
}
