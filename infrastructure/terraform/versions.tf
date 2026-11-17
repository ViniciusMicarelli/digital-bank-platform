terraform {
  required_version = ">= 1.6"
  required_providers {
    aws    = { source = "hashicorp/aws", version = "~> 5.80" }
    random = { source = "hashicorp/random", version = "~> 3.6" }
  }
  # Remote state with locking; configure per environment:
  #   terraform init -backend-config=env/prod.backend.hcl
  backend "s3" {}
}

provider "aws" {
  region = var.region
  default_tags {
    tags = { Project = "digital-bank", Environment = var.environment, ManagedBy = "terraform" }
  }
}
