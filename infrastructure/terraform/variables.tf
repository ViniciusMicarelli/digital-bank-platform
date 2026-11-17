variable "region" {
  type    = string
  default = "sa-east-1" # São Paulo: data residency for a Brazilian bank (LGPD)
}

variable "environment" {
  type = string
  validation {
    condition     = contains(["staging", "prod"], var.environment)
    error_message = "environment must be staging or prod"
  }
}

variable "vpc_cidr" {
  type    = string
  default = "10.40.0.0/16"
}

variable "kubernetes_version" {
  type    = string
  default = "1.33"
}

variable "node_instance_types" {
  type    = list(string)
  default = ["m7g.large"]
}

variable "db_instance_class" {
  type    = string
  default = "db.r7g.large"
}

variable "kafka_instance_type" {
  type    = string
  default = "kafka.m7g.large"
}

locals {
  name = "digital-bank-${var.environment}"
  azs  = slice(data.aws_availability_zones.available.names, 0, 3)
  prod = var.environment == "prod"
}

data "aws_availability_zones" "available" {
  state = "available"
}
