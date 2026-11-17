module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.31"

  cluster_name    = local.name
  cluster_version = var.kubernetes_version

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  cluster_endpoint_public_access = false
  enable_irsa                    = true

  # Kubernetes secrets encrypted with our key.
  cluster_encryption_config = { resources = ["secrets"], provider_key_arn = aws_kms_key.data.arn }

  eks_managed_node_groups = {
    services = {
      instance_types = var.node_instance_types
      ami_type       = "AL2023_ARM_64_STANDARD"
      min_size       = local.prod ? 3 : 2
      max_size       = local.prod ? 12 : 4
      desired_size   = local.prod ? 3 : 2
    }
  }

  # NetworkPolicies from the Helm chart need a CNI that enforces them.
  cluster_addons = {
    vpc-cni = {
      most_recent          = true
      configuration_values = jsonencode({ enableNetworkPolicy = "true" })
    }
    coredns    = { most_recent = true }
    kube-proxy = { most_recent = true }
  }
}
