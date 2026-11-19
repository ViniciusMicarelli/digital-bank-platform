# Deployment and Operations

## Environments

| | Local | Cloud (reference) |
|---|---|---|
| Orchestration | Docker Compose (`make up`) | Kubernetes (EKS) with the Helm chart |
| PostgreSQL | One container, one database per service | RDS PostgreSQL 16, Multi-AZ, encrypted with KMS, forced TLS, 30-day PITR |
| Kafka | Single KRaft broker | MSK, 3 brokers, TLS + IAM, encrypted at rest |
| Redis | Container | ElastiCache, TLS + auth token, encrypted at rest |
| Key protection | `KMS_ROOT_KEY` env (dev value) | AWS KMS key (`app_kms_key_arn`), never leaves KMS |
| mTLS certificates | `make certs` / `make rotate-certs` | cert-manager, 30-day certificates renewed 10 days before expiry |

## Kubernetes (Helm)

`infrastructure/kubernetes/digital-bank`: one chart for every service.

- **Deployments**: non-root (UID 65532), read-only root filesystem, all capabilities dropped, `RuntimeDefault` seccomp, no service-account token. Rolling updates with `maxUnavailable: 0`, spread across zones.
- **Probes**: the images are `FROM scratch`, so start-up, readiness and liveness run `/server healthcheck`, which calls the service's own gRPC health check over mTLS (or `/healthz` for the gateway).
- **HPA** (CPU 70 %, 2–10 replicas) and **PodDisruptionBudgets** for every service. kms-service runs with fixed replicas.
- **NetworkPolicies**: default deny, then each service admits exactly its mTLS callers (the same list as `ALLOWED_CALLERS`), plus Prometheus on the metrics port and the ingress controller for the gateway. Network and identity checks are independent layers.
- **Certificates**: cert-manager `Certificate` per workload with URI SAN `spiffe://digital-bank/<service>`. Services reload renewed certificates without restarting.
- **Secrets**: database URLs and the KMS root reference come from pre-existing Secrets (External Secrets / CSI driver), never from values.

```bash
make k8s-validate     # helm lint + template + kubeconform (also in CI)
helm upgrade --install db infrastructure/kubernetes/digital-bank -n bank --set image.tag=<version>
```

## Terraform (AWS)

`infrastructure/terraform`: VPC over 3 AZs with flow logs, private EKS endpoint with secrets encryption and a network-policy-enforcing CNI, RDS, MSK, ElastiCache and two customer-managed KMS keys with yearly rotation. Region `sa-east-1` keeps data in Brazil (LGPD).

```bash
make tf-validate      # fmt + validate, no credentials needed (also in CI)
terraform -chdir=infrastructure/terraform init -backend-config=env/prod.backend.hcl
terraform -chdir=infrastructure/terraform plan -var-file=env/prod.tfvars
```

## Keys (kms-service)

Envelope encryption: identity-service (signing keys, TOTP seeds), account-service (CPFs) and card-service (vault) each keep a **data key wrapped by kms-service** in their own `data_keys` table and unwrap it at startup. The KMS authorizes each key by the caller's mTLS identity (`account-service/*` only for account-service).

| Task | How |
|---|---|
| Rotate a master key | `RotateKey` (operator identity). Old versions keep decrypting; each service re-wraps its data key under the new version on its next start. |
| Import a pre-KMS key | Start the service with `KMS_ADDR` *and* the old `*_KEK` env once; the key is wrapped by the KMS (data stays readable). Then remove the env var. |
| Lose the KMS | Services keep running with the keys they already hold; new instances cannot start until the KMS is back. Run it with several replicas. |

## Certificates

- Locally, `make rotate-certs` reissues every workload certificate with the existing CA. Services check the files every 5 s and switch to the new certificate; existing connections are not dropped.
- `make certs` creates the CA only once, then issues certificates for new workloads only. `devcerts -force` starts a new CA (restart everything afterwards).

## Background Jobs

| Job | Where | Interval | What |
|---|---|---|---|
| Outbox relay | every service with an outbox | continuous | Publishes events to Kafka |
| Saga recovery | transaction-service | 5 s | Resumes stuck sagas; compensates reservations past their timeout (Pix 15 min, cards 7 days) |
| Pix recovery | pix-service | 5 s | Resumes transfers stuck mid-flow |
| Orphan reservations | card-service | 5 s | Releases reservations of authorizations answered `91` |
| KYC retries | user-service | 10 s | Asks the bureau again for verifications left pending |
| Savings yield | savings-service | 30 s | Pays each box the yield of every Brasília day that ended (exactly once, catching up), settles pending movements |
| Approval recovery | backoffice-service | 30 s | Expires four-eyes requests after 24 h; retries approved actions that did not settle |
| Notification deliveries | notification-service | 1 s | Sends pending pushes and e-mails (simulated), up to 5 attempts |
| Boleto recovery | payment-service | 5 s | Resumes payments stuck mid-flow; runs scheduled payments on their day |
| Invoice cycle | credit-service | 30 s | Closes invoices, moves unpaid ones to the next with charges, posts charges, expires holds, settles stuck payments |
| Ledger reconciliation | ledger-service | 5 min | Re-checks every invariant; `ledger_invariant_violations` must stay 0 |
| Retention | every service | 1 h | Deletes published outbox rows (7 d), processed inbox rows (30 d), expired refresh tokens |
| Key rotation | identity-service | daily check | Rotates the JWT signing key after 30 days |

## Resilience Built In

- gRPC clients wait for a restarting dependency instead of failing fast, reconnect within 3 s, re-resolve DNS after 1 s (pods change IPs) and bound every call to 10 s.
- Kafka consumers leave their group on shutdown, so a new pod takes over partitions at once (10 s session timeout if a pod dies).
- Every write path is idempotent and every multi-step flow is resumable, so a pod can be killed at any point.

## Runbook (Alerts)

| Alert | First steps |
|---|---|
| `LedgerInvariantViolated` | Stop money movement (block the gateway's write routes), find the account with the reconciliation query in `ledger-service`, compare `balances` with `SUM(postings)`, fix with a reversal entry, never an UPDATE. |
| `OutboxRelayFailing` | Check Kafka health and the service's relay logs; events wait safely in the outbox. |
| `EventsDeadLettered` | Read the `<topic>.dlq` record and its `error` header; fix the consumer, then replay the record to the topic. |
| `HighGrpcErrorRate` | Find the method in Grafana, then the trace in Jaeger by `trace_id` from the logs. |
| `SlowLedger` | Check lock waits (`pg_stat_activity`), hot accounts and RDS Performance Insights. |
