output "cluster_name" { value = module.eks.cluster_name }
output "postgres_endpoint" { value = aws_db_instance.postgres.address }
output "postgres_master_secret_arn" { value = aws_db_instance.postgres.master_user_secret[0].secret_arn }
output "kafka_bootstrap_brokers_iam" { value = aws_msk_cluster.kafka.bootstrap_brokers_sasl_iam }
output "redis_endpoint" { value = aws_elasticache_replication_group.redis.primary_endpoint_address }
output "app_kms_key_arn" { value = aws_kms_key.app.arn }
