# Customer-managed keys with automatic yearly rotation. The kms-service root
# key is generated inside AWS KMS and never leaves it (production replacement
# for KMS_ROOT_KEY); data stores are encrypted with their own keys.
resource "aws_kms_key" "app" {
  description             = "${local.name} kms-service root key"
  enable_key_rotation     = true
  deletion_window_in_days = 30
}

resource "aws_kms_alias" "app" {
  name          = "alias/${local.name}-app"
  target_key_id = aws_kms_key.app.key_id
}

resource "aws_kms_key" "data" {
  description             = "${local.name} RDS, MSK, ElastiCache and secrets"
  enable_key_rotation     = true
  deletion_window_in_days = 30
}

resource "aws_kms_alias" "data" {
  name          = "alias/${local.name}-data"
  target_key_id = aws_kms_key.data.key_id
}
