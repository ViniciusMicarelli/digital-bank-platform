# --- PostgreSQL: one instance, one database per service (ADR-0003). -----
resource "aws_security_group" "db" {
  name   = "${local.name}-db"
  vpc_id = module.vpc.vpc_id
  ingress {
    description     = "PostgreSQL from EKS nodes"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [module.eks.node_security_group_id]
  }
}

resource "aws_db_parameter_group" "postgres" {
  name   = "${local.name}-pg16"
  family = "postgres16"
  parameter {
    name  = "rds.force_ssl"
    value = "1"
  }
  parameter {
    name  = "log_min_duration_statement"
    value = "250"
  }
}

resource "aws_db_instance" "postgres" {
  identifier            = local.name
  engine                = "postgres"
  engine_version        = "16"
  instance_class        = var.db_instance_class
  allocated_storage     = 100
  max_allocated_storage = 1000
  storage_type          = "gp3"
  storage_encrypted     = true
  kms_key_id            = aws_kms_key.data.arn

  username                      = "bank"
  manage_master_user_password   = true # Secrets Manager, rotated
  master_user_secret_kms_key_id = aws_kms_key.data.arn

  db_subnet_group_name   = module.vpc.database_subnet_group_name
  vpc_security_group_ids = [aws_security_group.db.id]
  parameter_group_name   = aws_db_parameter_group.postgres.name

  multi_az                        = local.prod
  backup_retention_period         = local.prod ? 30 : 7 # point-in-time recovery window
  deletion_protection             = local.prod
  skip_final_snapshot             = !local.prod
  final_snapshot_identifier       = local.prod ? "${local.name}-final" : null
  performance_insights_enabled    = true
  performance_insights_kms_key_id = aws_kms_key.data.arn
  auto_minor_version_upgrade      = true
}

# --- Kafka (MSK): TLS in transit, IAM auth, encrypted at rest. ------------
resource "aws_security_group" "kafka" {
  name   = "${local.name}-kafka"
  vpc_id = module.vpc.vpc_id
  ingress {
    description     = "Kafka (TLS / IAM) from EKS nodes"
    from_port       = 9094
    to_port         = 9098
    protocol        = "tcp"
    security_groups = [module.eks.node_security_group_id]
  }
}

resource "aws_msk_cluster" "kafka" {
  cluster_name           = local.name
  kafka_version          = "3.9.x"
  number_of_broker_nodes = 3

  broker_node_group_info {
    instance_type   = var.kafka_instance_type
    client_subnets  = module.vpc.private_subnets
    security_groups = [aws_security_group.kafka.id]
    storage_info {
      ebs_storage_info { volume_size = 500 }
    }
  }

  encryption_info {
    encryption_at_rest_kms_key_arn = aws_kms_key.data.arn
    encryption_in_transit {
      client_broker = "TLS"
      in_cluster    = true
    }
  }

  client_authentication {
    sasl { iam = true }
  }
}

# --- Redis (ElastiCache): shared rate limits. ----------------------------
resource "aws_security_group" "redis" {
  name   = "${local.name}-redis"
  vpc_id = module.vpc.vpc_id
  ingress {
    description     = "Redis from EKS nodes"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [module.eks.node_security_group_id]
  }
}

resource "aws_elasticache_subnet_group" "redis" {
  name       = local.name
  subnet_ids = module.vpc.private_subnets
}

resource "random_password" "redis" {
  length  = 48
  special = false
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id       = local.name
  description                = "Gateway rate limits"
  engine                     = "redis"
  engine_version             = "7.1"
  node_type                  = "cache.t4g.medium"
  num_cache_clusters         = local.prod ? 2 : 1
  automatic_failover_enabled = local.prod
  multi_az_enabled           = local.prod
  subnet_group_name          = aws_elasticache_subnet_group.redis.name
  security_group_ids         = [aws_security_group.redis.id]
  at_rest_encryption_enabled = true
  kms_key_id                 = aws_kms_key.data.arn
  transit_encryption_enabled = true
  auth_token                 = random_password.redis.result
}
