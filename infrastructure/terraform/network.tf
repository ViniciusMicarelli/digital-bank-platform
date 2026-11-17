# Three AZs; workloads and data stores live in private subnets only.
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.16"

  name = local.name
  cidr = var.vpc_cidr
  azs  = local.azs

  private_subnets  = [for i in range(3) : cidrsubnet(var.vpc_cidr, 4, i)]
  public_subnets   = [for i in range(3) : cidrsubnet(var.vpc_cidr, 8, 48 + i)]
  database_subnets = [for i in range(3) : cidrsubnet(var.vpc_cidr, 8, 64 + i)]

  enable_nat_gateway     = true
  one_nat_gateway_per_az = local.prod
  enable_dns_hostnames   = true

  create_database_subnet_group = true

  public_subnet_tags  = { "kubernetes.io/role/elb" = 1 }
  private_subnet_tags = { "kubernetes.io/role/internal-elb" = 1 }

  enable_flow_log                      = true
  create_flow_log_cloudwatch_log_group = true
  create_flow_log_cloudwatch_iam_role  = true
}
