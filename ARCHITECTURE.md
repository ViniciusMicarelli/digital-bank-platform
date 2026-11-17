# Architecture

## System Architecture

### Overview

The Digital Bank Platform is a distributed, event-driven microservices architecture designed for high availability, horizontal scalability, and regulatory compliance. The system follows a layered architecture with clear separation of concerns between security boundaries, business logic, and data management.

```
┌──────────────────────────────────────────────────────────────────────┐
│                         CLIENT LAYER                                │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐            │
│   │   Mobile App │  │   Web Portal  │  │   API Client  │            │
│   │              │  │              │  │              │            │
│   └──────┬───────┘  └──────┬───────┘  └──────┬───────┘            │
│          │                 │                 │                      │
│          └─────────────────┼─────────────────┘                      │
└────────────────────────────┼───────────────────────────────────────┘
                             │
┌────────────────────────────┴───────────────────────────────────────┐
│                        API GATEWAY                                  │
│   ┌──────────────────────────────────────────────────────────────┐ │
│   │                   Kong / OpenResty                            │ │
│   │   ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │ │
│   │   │  Auth   │  │  Rate   │  │  Limit  │  │  Route   │        │ │
│   │   │  ZAP    │  │  Limit  │  │  ZAP    │  │  Tables  │        │ │
│   │   └─────────┘  └─────────┘  └─────────┘  └─────────┘        │ │
│   └──────────────────────────────────────────────────────────────┘ │
│   ┌──────────────────────────────────────────────────────────────┐ │
│   │                        mTLS                                  │ │
│   │                    Mutual TLS Authentication                 │ │
│   └──────────────────────────────────────────────────────────────┘ │
└────────────────────────────┬───────────────────────────────────────┘
                             │
┌────────────────────────────┴───────────────────────────────────────┐
│                        APPLICATION LAYER                           │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                   SERVICE MESH                               │ │
│  │       (gRPC with mTLS, service discovery)                   │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐        │
│  │   Auth       │  │   Ledger       │  │   Payments     │        │
│  │   Service     │  │   Service      │  │   Service       │        │
│  │   ───────────│  │   ───────────  │  │   ───────────  │        │
│  │  /auth      │  │  /ledger      │  │  /payments     │        │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘        │
│         │                 │                 │                 │
│         ▼                 ▼                 ▼                 │
│  ┌──────┴───────┐  ┌──────┴───────┐  ┌──────┴───────┐        │
│  │   Accounts   │  │   Transactions│  │    Cards     │        │
│  │   Service    │  │   Store      │  │   Service     │        │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘        │
│         │                 │                 │                 │
│         │                 │                 │                 │
│         ▼                 ▼                 ▼                 │
│  ┌──────┴───────┐  ┌──────┴───────┐  ┌──────┴───────┐        │
│  │   Users      │  │   Transfers  │  │    Vault     │        │
│  │   Service    │  │   Service    │  │   (External) │        │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘        │
│         │                 │                 │                 │
│         └─────────────────┼─────────────────┘                 │
│                           │                                    │
│                           ▼                                    │
│                   ┌──────────────────┐                       │
│                   │    Event Bus     │                       │
│                   │    (Kafka/NSQ)    │                       │
│                   └────────┬─────────┘                       │
└────────────────────────────┼─────────────────────────────────────┘
                             │
┌────────────────────────────┴───────────────────────────────────────┐
│                         DATA LAYER                                 │
│                                                                  │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐        │
│  │   PostgreSQL  │  │    Redis       │  │   Vault       │        │
│  │   (Primary)   │  │   (Cache)      │  │   (Secrets)   │        │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘        │
│         │                 │                 │                 │
│         │                 │                 │                 │
│         ▼                 ▼                 ▼                 │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐        │
│  │  PostgreSQL   │  │   PostgreSQL   │  │   Vault       │        │
│  │  (Replica)    │  │  (Analytics)   │  │   (Backup)     │        │
│  └───────────────┘  └───────────────┘  └──────┬───────┘        │
│                                                  │              │
│                                                  ▼              │
│                                         ┌───────────────┐        │
│                                         │   Stripe      │        │
│                                         │   (External) │        │
│                                         └───────────────┘        │
└────────────────────────────┬─────────────────────────────────────┘
                             │
┌────────────────────────────┴───────────────────────────────────────┐
│                        INFRASTRUCTURE LAYER                        │
│                                                                  │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐        │
│  │   Kubernetes  │  │   Prometheus   │  │   Jaeger      │        │
│  │   Cluster     │  │   (Metrics)    │  │   (Tracing)   │        │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘        │
│         │                 │                 │                 │
│         ▼                 ▼                 ▼                 │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐        │
│  │   Load Balancer│ │    Grafana     │  │   PagerDuty   │        │
│  │   (Ingress)   │  │    (Dashboards)│  │   (Alerts)    │        │
│  └───────────────┘  └───────────────┘  └───────────────┘        │
└───────────────────────────────────────────────────────────────────┘
```

### Architecture Principles

1. **Domain-Driven Design (DDD)**: Services are organized around business capabilities with clear bounded contexts
2. **Event-Driven Architecture**: Services communicate asynchronously via events for loose coupling
3. **Saga Pattern**: Long-running transactions are coordinated through compensating transactions
4. **Circuit Breaker**: Graceful degradation during partial system failures
5. **Anti-Corruption Layer**: Protects services from incompatible external system formats

## Service Architecture

### Auth Service

**Purpose**: User authentication, authorization, and token management.

**Responsibilities**:
- User registration and login
- JWT token generation and validation
- Session management
- Two-factor authentication
- Permission and role management

**Endpoints**:
- `POST /auth/register` - Register new user
- `POST /auth/login` - User login
- `POST /auth/refresh` - Refresh access token
- `POST /auth/logout` - User logout
- `POST /auth/reset-password` - Password reset

**Tech Stack**: gRPC, Keycloak, PostgreSQL

### Accounts Service

**Purpose**: Account lifecycle management.

**Responsibilities**:
- Account creation (checking, savings, business)
- Account closure
- Account status management
- Account hierarchy (customer -> accounts)

**Endpoints**:
- `POST /accounts` - Create account
- `GET /accounts` - List accounts
- `GET /accounts/{id}` - Get account details
- `POST /accounts/{id}/close` - Close account
- `POST /accounts/{id}/freeze` - Freeze account

**Tech Stack**: gRPC, PostgreSQL, Redis (caching)

### Ledger Service

**Purpose**: Core ledger functionality with ACID compliance.

**Responsibilities**:
- Transaction recording
- Double-entry bookkeeping
- Account balance calculations
- Transaction audit trail

**Endpoints**:
- `POST /ledger/transactions` - Create transaction
- `GET /ledger/transactions` - List transactions
- `GET /ledger/accounts/{id}/balances` - Get balance history
- `POST /ledger/reversals` - Reverse transaction

**Tech Stack**: gRPC, PostgreSQL

### Payments Service

**Purpose**: Payment processing and card transactions.

**Responsibilities**:
- Payment initiation
- Card transaction processing
- Recurring payments
- Payment routing to external processors

**Endpoints**:
- `POST /payments` - Initiate payment
- `POST /payments/{id}/cancel` - Cancel payment
- `POST /cards/charge` - Charge card
- `POST /payments/recurring` - Recurring payment setup

**Tech Stack**: gRPC, PostgreSQL, Vault, Stripe (external)

### Cards Service

**Purpose**: Card lifecycle management.

**Responsibilities**:
- Virtual card creation
- Card provisioning
- Card pin generation
- Card cancellation
- Card expiration management

**Endpoints**:
- `POST /cards` - Create virtual card
- `GET /cards` - List cards
- `POST /cards/{id}/activate` - Activate card
- `POST /cards/{id}/freeze` - Freeze card
- `DELETE /cards/{id}` - Cancel card

**Tech Stack**: gRPC, Vault (for card data), PostgreSQL

### Transfers Service

**Purpose**: Inter-account transfers.

**Responsibilities**:
- Account-to-account transfers
- Transfers initiation and routing
- Transfer confirmation
- Transfer dispute handling

**Endpoints**:
- `POST /transfers` - Initiate transfer
- `GET /transfers` - List transfers
- `POST /transfers/{id}/confirm` - Confirm transfer
- `POST /transfers/{id}/dispute` - Dispute transfer

**Tech Stack**: gRPC, PostgreSQL, Ledger (validation)

### Users Service

**Purpose**: User and customer management.

**Responsibilities**:
- User profile management
- Customer onboarding
- KYC verification
- User role management

**Endpoints**:
- `POST /users` - Create user
- `GET /users` - List users
- `PUT /users/{id}` - Update user
- `POST /users/{id}/kyc` - KYC verification

**Tech Stack**: gRPC, PostgreSQL

### Notification Service

**Purpose**: Multi-channel notification delivery.

**Responsibilities**:
- Email notifications
- SMS notifications
- Push notifications
- Notification templates

**Endpoints**:
- `POST /notifications` - Send notification
- `POST /notifications/templates` - Create template
- `GET /notifications/{id}` - Get notification

**Tech Stack**: gRPC, Redis, SMTP, Twilio (external)

## Service Dependencies

### Direct Dependencies

```
┌───────────────────────────────────────────────────────────────┐
│                        Auth Service                           │
│   └─ Keycloak (Identity Management)                          │
│   └─ PostgreSQL (User data, tokens)                          │
│   └─ Redis (Session cache)                                   │
└───────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────┐
│                      Accounts Service                         │
│   └─ PostgreSQL (Account data)                                │
│   └─ Redis (Account cache)                                    │
│   └─ Ledger Service (Balance calculations)                   │
└───────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────┐
│                     Ledger Service                            │
│   └─ PostgreSQL (Ledger tables)                               │
│   └─ Accounts Service (Account validation)                   │
└───────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────┐
│                   Payments Service                            │
│   └─ PostgreSQL (Payment records)                             │
│   └─ Vault (Card data, encryption keys)                      │
│   └─ Stripe (External payment processing)                   │
│   └─ Ledger Service (Payment validation)                     │
└───────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────┐
│                     Cards Service                             │
│   └─ Vault (Card data, encryption keys)                      │
│   └─ PostgreSQL (Card metadata)                              │
│   └─ Payments Service (Payment routing)                      │
│   └─ Stripe (Card processing)                               │
└───────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────┐
│                   Transfers Service                           │
│   └─ PostgreSQL (Transfer records)                           │
│   └─ Ledger Service (Transfer validation)                    │
│   └─ Events (Transaction events)                             │
└───────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────┐
│                    Users Service                              │
│   └─ PostgreSQL (User data)                                  │
│   └─ Keycloak (Role management)                              │
│   └─ Accounts Service (Account creation)                     │
└───────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────┐
│                Notification Service                           │
│   └─ Redis (Notification queue)                              │
│   └─ PostgreSQL (Notification logs)                         │
│   └─ SMTP / Twilio (External delivery)                       │
└───────────────────────────────────────────────────────────────┘
```

### Event Dependencies

```
┌───────────────────────────────────────────────────────────────┐
│                  Event Bus (Kafka)                            │
│   └─ Auth events → Users service                              │
│   └─ Account events → Ledger service                         │
│   └─ Transaction events → Payments service                   │
│   └─ Payment events → Cards service                          │
│   └─ Card events → Transfers service                         │
│   └─ Transfer events → Notification service                  │
└───────────────────────────────────────────────────────────────┘
```

## Data Flow

### User Registration Flow

```
1. User submits registration request to API Gateway
2. API Gateway validates request, forwards to Auth Service
3. Auth Service validates against Keycloak
4. Auth Service creates user record in PostgreSQL
5. Auth Service emits UserCreated event
6. Users Service receives event, creates user profile
7. Notification Service sends welcome email
8. Account Service creates default accounts
```

### Account Creation Flow

```
1. User authenticates via API Gateway
2. Auth Service validates JWT token
3. API Gateway validates token, creates session
4. User requests new account
5. Accounts Service validates user permissions
6. Accounts Service creates account record in PostgreSQL
7. Accounts Service emits AccountCreated event
8. Ledger Service creates initial balance transaction
9. Users Service updates user account mapping
10. Notification Service sends confirmation
```

### Transaction Processing Flow

```
1. User initiates transaction request
2. API Gateway validates request
3. Auth Service validates user's JWT token
4. Request forwarded to appropriate service
5. Service validates account, balance, permissions
6. Ledger Service creates transaction record
7. Ledger Service emits TransactionCreated event
8. Payments Service validates payment routing (if payment)
9. External processors (Stripe) handle payment
10. Events propagate to dependent services
11. Notification Service sends confirmation
```

### Payment Processing Flow

```
1. Payment request received by API Gateway
2. API Gateway validates authentication
3. Request routed to Payments Service
4. Payments Service validates card via Vault
5. Payments Service initiates with Stripe
6. Stripe responds with payment status
7. Payments Service updates payment record
8. Payments Service emits PaymentCompleted event
9. Ledger Service records the payment
10. Cards Service updates card balance
11. Transfers Service routes payment (if transfer)
12. Users Service updates user transaction history
13. Notification Service sends payment confirmation
```

### Card Dispute Flow

```
1. User initiates card dispute
2. API Gateway routes to Cards Service
3. Cards Service validates user permission
4. Cards Service creates dispute record
5. Cards Service emits CardDisputed event
6. Cards Service triggers Vault revocation
7. Vault Service revokes card keys
8. Payments Service cancels pending payments
9. Transfers Service cancels transfers
10. Notification Service sends dispute confirmation
```

## Database Schema Design

### Core Entities

#### Accounts
```sql
CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    account_number VARCHAR(17) NOT NULL,
    customer_id UUID NOT NULL,
    type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    balance DECIMAL(19,2) NOT NULL DEFAULT 0,
    daily_limit DECIMAL(19,2),
    monthly_limit DECIMAL(19,2),
    country VARCHAR(2),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(account_number)
);

CREATE TABLE accounts_transactions (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL,
    transaction_id UUID NOT NULL,
    created_at TIMESTAMP NOT NULL,
    UNIQUE(account_id, transaction_id)
);
```

#### Transactions (Ledger)
```sql
CREATE TABLE transactions (
    id UUID PRIMARY KEY,
    transaction_type VARCHAR(20) NOT NULL,
    debit_account_id UUID,
    credit_account_id UUID,
    debit_amount DECIMAL(19,2),
    credit_amount DECIMAL(19,2),
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,
    reference VARCHAR(50),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE transactions_ledger_entries (
    id UUID PRIMARY KEY,
    transaction_id UUID NOT NULL,
    account_id UUID NOT NULL,
    entry_type VARCHAR(20) NOT NULL,
    amount DECIMAL(19,2) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    balance_after DECIMAL(19,2) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    UNIQUE(transaction_id, account_id)
);
```

#### Cards
```sql
CREATE TABLE cards (
    id UUID PRIMARY KEY,
    card_number_encrypted VARCHAR(4) NOT NULL, -- Last 4 digits
    card_holder_name VARCHAR(100),
    card_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    expiry_date DATE NOT NULL,
    pin_encrypted VARCHAR(4) NOT NULL,
    encrypted_data VARCHAR(1024),
    vault_key_id VARCHAR(50) NOT NULL,
    customer_id UUID NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE card_transactions (
    id UUID PRIMARY KEY,
    card_id UUID NOT NULL,
    transaction_id UUID NOT NULL,
    type VARCHAR(20) NOT NULL,
    amount DECIMAL(19,2) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    UNIQUE(card_id, transaction_id)
);
```

#### Payments
```sql
CREATE TABLE payments (
    id UUID PRIMARY KEY,
    reference VARCHAR(50) NOT NULL,
    payer_id UUID NOT NULL,
    payee_id UUID,
    amount DECIMAL(19,2) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,
    payment_method VARCHAR(20),
    processing_provider VARCHAR(50),
    provider_reference VARCHAR(255),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE payments_transfers (
    id UUID PRIMARY KEY,
    payment_id UUID NOT NULL,
    transfer_id UUID,
    created_at TIMESTAMP NOT NULL,
    UNIQUE(payment_id)
);
```

#### Users
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    phone VARCHAR(20),
    avatar_url VARCHAR(255),
    status VARCHAR(20) NOT NULL,
    kyc_status VARCHAR(20),
    kyc_verified_at TIMESTAMP,
    role VARCHAR(50),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE users_accounts (
    user_id UUID NOT NULL,
    account_id UUID NOT NULL,
    primary_account BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, account_id)
);
```

#### Sessions
```sql
CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    device_info JSONB,
    expires_at TIMESTAMP NOT NULL,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
```

### Database Architecture

#### Primary Database (PostgreSQL)
- Type: PostgreSQL 15
- Storage: Primary + Replica
- Purpose: OLTP transactions
- Sharding: By customer ID range
- Indexing: Partial indexes for status filtering

#### Analytics Database (PostgreSQL)
- Type: PostgreSQL 15
- Storage: Dedicated cluster
- Purpose: Reporting, analytics
- Sync: CDC via Debezium
- Data Retention: 5 years

#### Cache Layer (Redis)
- Type: Redis 7.x
- Purpose: Session caching
- Account balance caching
- Rate limit storage
- Notification queues

### Database Design Principles

1. **ACID Compliance**: Core ledger uses ACID transactions
2. **Idempotency**: All payment/transfer operations support idempotency tokens
3. **Audit Trail**: Every state change recorded with timestamp and actor
4. **Data Types**: Use DECIMAL for financial amounts
5. **Soft Deletes**: Records marked deleted rather than removed
6. **JSONB**: Flexible metadata stored as JSONB
7. **Partitioning**: Large tables (transactions, logs) partitioned by date

## API Contracts

### Authentication Service (gRPC)

```protobuf
// api/auth/proto/auth.proto

syntax = "proto3";

package digitalbank.auth.v1;

service AuthService {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc RefreshToken(RefreshRequest) returns (RefreshResponse);
  rpc Logout(LogoutRequest) returns (LogoutResponse);
  rpc ValidateToken(ValidateRequest) returns (ValidateResponse);
}

message RegisterRequest {
  string username = 1;
  string email = 2;
  string password = 3;
  string password_confirm = 4;
}

message RegisterResponse {
  string user_id = 1;
  string access_token = 2;
  string refresh_token = 3;
  string error = 4;
}

message LoginRequest {
  string username = 1;
  string password = 2;
  string refresh_token = 3;
  string otp_code = 4;
}

message LoginResponse {
  string user_id = 1;
  string access_token = 2;
  string refresh_token = 3;
  int32 expires_in = 4;
  map<string, string> claims = 5;
  string error = 6;
}

message RefreshRequest {
  string refresh_token = 1;
  string device_id = 2;
}

message RefreshResponse {
  string access_token = 1;
  string refresh_token = 2;
  int32 expires_in = 3;
  map<string, string> claims = 4;
  string error = 5;
}

service AuthorizationService {
  rpc CreateToken(CreateTokenRequest) returns (TokenResponse);
  rpc RevokeToken(RevokeTokenRequest) returns (RevokeTokenResponse);
  rpc ValidatePermission(ValidatePermissionRequest) returns (ValidatePermissionResponse);
}
```

### Accounts Service (gRPC)

```protobuf
// api/accounts/proto/accounts.proto

syntax = "proto3";

package digitalbank.accounts.v1;

service AccountsService {
  rpc CreateAccount(CreateAccountRequest) returns (CreateAccountResponse);
  rpc GetAccounts(GetAccountsRequest) returns (GetAccountsResponse);
  rpc GetAccount(GetAccountRequest) returns (GetAccountResponse);
  rpc CloseAccount(CloseAccountRequest) returns (CloseAccountResponse);
  rpc FreezeAccount(FreezeAccountRequest) returns (FreezeAccountResponse);
  rpc UnfreezeAccount(UnfreezeAccountRequest) returns (UnfreezeAccountResponse);
}

message CreateAccountRequest {
  string customer_id = 1;
  string account_type = 2;
  string currency = 3;
  string account_number = 4;
  AccountSettings settings = 5;
  string idempotency_key = 6;
}

message CreateAccountResponse {
  string account_id = 1;
  string account_number = 2;
  string currency = 3;
  decimal initial_balance = 4;
  string status = 5;
  string error = 6;
}

message GetAccountsRequest {
  string customer_id = 1;
  string filter_status = 2;
  string filter_type = 3;
  int32 page = 4;
  int32 page_size = 5;
}

message GetAccountsResponse {
  repeated Account accounts = 1;
  int32 total_count = 2;
  string error = 3;
}

message Account {
  string id = 1;
  string account_number = 2;
  string account_type = 3;
  string status = 4;
  string currency = 5;
  decimal balance = 6;
  decimal available_balance = 7;
  repeated AccountTransaction transactions = 8;
  AccountMetadata metadata = 9;
}

message CloseAccountRequest {
  string account_id = 1;
  string reason = 2;
  string idempotency_key = 3;
}

message CloseAccountResponse {
  string status = 1;
  string error = 2;
}
```

### Payments Service (gRPC)

```protobuf
// api/payments/proto/payments.proto

syntax = "proto3";

package digitalbank.payments.v1;

service PaymentsService {
  rpc InitiatePayment(InitiatePaymentRequest) returns (InitiatePaymentResponse);
  rpc GetPayment(GetPaymentRequest) returns (GetPaymentResponse);
  rpc GetPayments(GetPaymentsRequest) returns (GetPaymentsResponse);
  rpc CancelPayment(CancelPaymentRequest) returns (CancelPaymentResponse);
  rpc ValidatePayment(ValidatePaymentRequest) returns (ValidatePaymentResponse);
}

message InitiatePaymentRequest {
  string payer_id = 1;
  string payee_id = 2;
  decimal amount = 3;
  string currency = 4;
  PaymentMethod method = 5;
  string reference = 6;
  PaymentMetadata metadata = 7;
  string idempotency_key = 8;
}

message PaymentMethod {
  string type = 1;
  string card_id = 2;
  string external_reference = 3;
}

message InitiatePaymentResponse {
  string payment_id = 1;
  string status = 2;
  decimal current_amount = 3;
  string provider_reference = 4;
  string error = 5;
}

message GetPaymentRequest {
  string payment_id = 1;
  string reference = 2;
}

message GetPaymentResponse {
  string payment_id = 1;
  string status = 2;
  decimal paid_amount = 3;
  decimal current_amount = 4;
  repeated PaymentEvent events = 5;
  string error = 6;
}

message PaymentEvent {
  string event_id = 1;
  string event_type = 2;
  string status = 3;
  repeated string details = 4;
  int64 timestamp = 5;
}

service CardPaymentService {
  rpc ChargeCard(ChargeCardRequest) returns (ChargeCardResponse);
  rpc RefundCard(RefundCardRequest) returns (RefundCardResponse);
}

message ChargeCardRequest {
  string card_id = 1;
  string merchant_id = 2;
  decimal amount = 3;
  string currency = 4;
  string reference = 5;
  string idempotency_key = 6;
}

message ChargeCardResponse {
  string transaction_id = 1;
  string status = 2;
  decimal amount = 3;
  string error = 4;
}
```

### Ledger Service (gRPC)

```protobuf
// api/ledger/proto/ledger.proto

syntax = "proto3";

package digitalbank.ledger.v1;

service LedgerService {
  rpc CreateTransaction(CreateTransactionRequest) returns (CreateTransactionResponse);
  rpc CreatePayment(CreatePaymentRequest) returns (CreatePaymentResponse);
  rpc CreateTransfer(CreateTransferRequest) returns (CreateTransferResponse);
  rpc GetTransactions(GetTransactionsRequest) returns (GetTransactionsResponse);
  rpc GetAccountBalances(GetAccountBalancesRequest) returns (GetAccountBalancesResponse);
  rpc ReverseTransaction(ReverseTransactionRequest) returns (ReverseTransactionResponse);
}

message Transaction {
  string id = 1;
  string transaction_type = 2;
  string debit_account_id = 3;
  string credit_account_id = 4;
  decimal debit_amount = 5;
  decimal credit_amount = 6;
  string currency = 7;
  string status = 8;
  string reference = 9;
  repeated LedgerEntry entries = 10;
  string created_by = 11;
  google.protobuf.Timestamp created_at = 12;
  string metadata = 13;
}

message LedgerEntry {
  string id = 1;
  string transaction_id = 2;
  string account_id = 3;
  string entry_type = 4;
  decimal amount = 5;
  string currency = 6;
  decimal balance_before = 7;
  decimal balance_after = 8;
  google.protobuf.Timestamp created_at = 9;
}

message CreateTransactionRequest {
  string transaction_type = 1;
  string debit_account_id = 2;
  string credit_account_id = 3;
  decimal debit_amount = 4;
  decimal credit_amount = 5;
  string currency = 6;
  string reference = 7;
  string idempotency_key = 8;
}

message CreateTransactionResponse {
  string transaction_id = 1;
  repeated LedgerEntry entries = 2;
  bool balance_verified = 3;
  string error = 4;
}

message GetTransactionsRequest {
  string account_id = 1;
  string transaction_type = 2;
  string status = 3;
  string date_from = 4;
  string date_to = 5;
  string reference = 6;
  int32 page = 7;
  int32 page_size = 8;
}

message GetTransactionsResponse {
  repeated Transaction transactions = 1;
  int32 total_count = 2;
  repeated LedgerEntry balance_entries = 3;
  string error = 4;
}
```

### Cards Service (gRPC)

```protobuf
// api/cards/proto/cards.proto

syntax = "proto3";

package digitalbank.cards.v1;

service CardsService {
  rpc CreateCard(CreateCardRequest) returns (CreateCardResponse);
  rpc GetCards(GetCardsRequest) returns (GetCardsResponse);
  rpc GetCard(GetCardRequest) returns (GetCardResponse);
  rpc ActivateCard(ActivateCardRequest) returns (ActivateCardResponse);
  rpc FreezeCard(FreezeCardRequest) returns (FreezeCardResponse);
  rpc CancelCard(CancelCardRequest) returns (CancelCardResponse);
  rpc CreatePin(CreatePinRequest) returns (CreatePinResponse);
}

message Card {
  string id = 1;
  string card_number_encrypted = 2;
  string card_type = 3;
  string status = 4;
  string cardholder_name = 5;
  google.protobuf.Timestamp expiry_date = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp last_used_at = 8;
  CardMetadata metadata = 9;
  string vault_key_id = 10;
}

message CardMetadata {
  string card_brand = 1;
  string card_level = 2;
  repeated string card_features = 3;
  decimal daily_limit = 4;
  decimal monthly_limit = 5;
}

message CreateCardRequest {
  string customer_id = 1;
  string card_type = 2;
  string cardholder_name = 3;
  string vault_key_id = 4;
  CardMetadata metadata = 5;
}

message CreateCardResponse {
  string card_id = 1;
  string card_number_masked = 2;
  string expiry_date = 3;
  string card_number = 4;  // Encrypted, for display only
  string error = 5;
}

message CardRequest {
  string id = 1;
  string card_type = 2;
  string status = 3;
}
```

### Transfers Service (gRPC)

```protobuf
// api/transfers/proto/transfers.proto

syntax = "proto3";

package digitalbank.transfers.v1;

service TransfersService {
  rpc InitiateTransfer(InitiateTransferRequest) returns (InitiateTransferResponse);
  rpc ConfirmTransfer(ConfirmTransferRequest) returns (ConfirmTransferResponse);
  rpc CancelTransfer(CancelTransferRequest) returns (CancelTransferResponse);
  rpc DisputeTransfer(DisputeTransferRequest) returns (DisputeTransferResponse);
  rpc GetTransfers(GetTransfersRequest) returns (GetTransfersResponse);
}

message InitiateTransferRequest {
  string sender_account_id = 1;
  string receiver_account_id = 2;
  decimal amount = 3;
  string currency = 4;
  string reference = 5;
  TransferMetadata metadata = 6;
  string idempotency_key = 7;
}

message TransferMetadata {
  string receiver_name = 1;
  string receiver_account_type = 2;
  string reason = 3;
  repeated string items = 4;
}

message InitiateTransferResponse {
  string transfer_id = 1;
  string status = 2;
  decimal fee = 3;
  decimal net_amount = 4;
  string error = 5;
}

message ConfirmTransferRequest {
  string transfer_id = 1;
  string confirmation_code = 2;
}

message ConfirmTransferResponse {
  string status = 1;
  string error = 2;
}

message DisputeTransferRequest {
  string transfer_id = 1;
  string dispute_reason = 2;
  string evidence = 3;
}

message DisputeTransferResponse {
  string dispute_id = 1;
  string status = 2;
  string adjudicator = 3;
  string error = 4;
}
```

## Deployment Topology

### Kubernetes Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                       Load Balancer                             │
│   (nginx / AWS ALB / Cloudflare)                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   API Gateway Layer                             │
│   ┌───────────────────────────────────────────────────────┐    │
│   │              Kong Gateway                             │    │
│   │              ─────────────────                       │    │
│   │    Route Tables │ Rate Limits │ Plugins (JWT)       │    │
│   │              ─────────────────                       │    │
│   │              ─────────────────                       │    │
│   └───────────────────────────────────────────────────────┘    │
│   ┌───────────────────────────────────────────────────────┐    │
│   │            OpenResty (nginx_lua)                     │    │
│   │          ──────────────────────────────────         │    │
│   │          ──────────────────────────────────         │    │
│   └───────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼ (mTLS)
                    ┌─────────────────────────────────────────┐
                    │              Service Mesh                │
                    │              (Istio)                    │
                    │    Mutual TLS                         │   │
                    │    Rate Limiting                     │   │
                    │    Circuit Breaker                   │   │
                    │    Service Discovery                 │   │
                    │    ──────────────────────            │   │
                    │              ────────                │   │
                    └─────────────────────────────────────────┘
                              │
                    ┌─────────┼─────────┐
                    │         │         │
                    ▼         ▼         ▼
         ┌──────────┐  ┌──────────┐  ┌──────────┐
         │  Auth    │  │   Ledger  │  │  Payments│
         │  Pod     │  │   Pod     │  │   Pod    │
         └────┬─────┘  └────┬─────┘  └────┬─────┘
              │             │             │
              ▼             ▼             ▼
         ┌──────────┐  ┌──────────┐  ┌──────────┐
         │ Accounts │  │ Accounts │  │    Cards │
         │ Pod      │  │   Pod    │  │   Pod    │
         └────┬─────┘  └────┬─────┘  └────┬─────┘
              │             │             │
              ▼             ▼             ▼
         ┌──────────┐  ┌──────────┐  ┌──────────┐
         │  Users   │  │ Transfers │  │  Vault   │
         │ Pod      │  │  Pod     │  │ (Secrets) │
         └────┬─────┘  └────┬─────┘  └────┬─────┘
              │             │             │
              └─────────────┼─────────────┘
                            ▼
                     ┌──────────┐
                     │ Event    │
                     │  Kafka   │
                     └─────┬────┘
                          │
                          ▼
                    ┌──────────┐
                    │  Redis   │
                    │ Pod      │
                    └─────┬────┘
                         │
                         ▼
                   ┌──────────┐
                   │PostgreSQL│
                   │  Cluster  │
                   └─────┬────┘
                        │
        ┌───────────────┼───────────────┐
        ▼              ▼              ▼
   ┌─────────┐  ┌─────────┐  ┌─────────┐
   │PostgreSQL│  │PostgreSQL│  │Vault   │
   │ Replica  │  │Analytics │  │ Server  │
   └─────────┘  └─────────┘  └─────────┘
```

### Kubernetes Resources

#### Deployment
```yaml
# api/ledger/k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ledger
spec:
  replicas: 3
  selector:
    matchLabels:
      app: ledger
  template:
    metadata:
      labels:
        app: ledger
        version: "v1.0.0"
    spec:
      containers:
        - name: ledger
          image: digitalbank/ledger:latest
          resources:
            requests:
              cpu: 500m
              memory: 512Mi
            limits:
              cpu: 1
              memory: 1Gi
          ports:
            - containerPort: 50051
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: db-credentials
                  key: database-url
          livenessProbe:
            tcpSocket:
              port: 50051
            initialDelaySeconds: 15
            periodSeconds: 10
          readinessProbe:
            tcpSocket:
              port: 50051
            initialDelaySeconds: 5
            periodSeconds: 5
```

#### StatefulSet (PostgreSQL)
```yaml
# infrastructure/k8s/db/storedb-postgresql.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgresql
spec:
  serviceName: postgresql
  replicas: 1
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
        - name: postgresql
          image: postgres:15
          resources:
            requests:
              cpu: 1
              memory: 2Gi
            limits:
              cpu: 2
              memory: 4Gi
          env:
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: db-secrets
                  key: postgres-password
            - name: POSTGRES_DB
              value: "digitalbank"
            - name: PGDATA
              value: "/var/lib/postgresql/data"
          ports:
            - containerPort: 5432
          volumeMounts:
            - name: postgres-storage
              mountPath: /var/lib/postgresql/data
      volumes:
        - name: postgres-storage
          persistentVolumeClaim:
            claimName: postgres-pvc
          persistentVolumeReclaimPolicy: Retain
  volumeClaimTemplates:
    - metadata:
        name: postgres-pvc
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: "gp2"
        resources:
          requests:
            storage: 100Gi
```

#### Service
```yaml
# infrastructure/k8s/svc/ledger-svc.yaml
apiVersion: v1
kind: Service
metadata:
  name: ledger-svc
  labels:
    app: ledger
spec:
  type: ClusterIP
  selector:
    app: ledger
  ports:
    - name: grpc
      port: 50051
      targetPort: 50051
    - name: http-debug
      port: 8080
      targetPort: 8080
      protocol: TCP
```

#### Network Policy
```yaml
# infrastructure/k8s/network-policies.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: ledger-network-policy
spec:
  podSelector:
    matchLabels:
      app: ledger
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: api-gateway
      ports:
        - protocol: TCP
          port: 50051
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - podSelector:
            matchLabels:
              app: postgres
      ports:
        - protocol: TCP
          port: 5432
    - to:
        - podSelector:
            matchLabels:
              app: accounts
      ports:
        - protocol: TCP
          port: 50051
```

### Infrastructure Layers

#### Tier 1: Client Layer
- Mobile Apps (iOS/Android)
- Web Portal
- API Clients (gRPC/REST)

#### Tier 2: Gateway Layer
- Kong Gateway (API Gateway)
- OpenResty (nginx)
- API Routes
- Rate Limiting
- Authentication

#### Tier 3: Application Layer
- Auth Service
- Accounts Service
- Ledger Service
- Payments Service
- Cards Service
- Transfers Service
- Users Service
- Notification Service

#### Tier 4: Data Layer
- PostgreSQL (Primary)
- PostgreSQL (Replica)
- PostgreSQL (Analytics)
- Redis Cluster
- Vault Server
- Kafka Cluster
- Event Bus

#### Tier 5: Infrastructure Layer
- Kubernetes Cluster
- Load Balancer
- Monitoring (Prometheus/Grafana)
- Tracing (Jaeger)
- Logging (ELK/Loki)

## Security Architecture

### Authentication & Authorization

#### Token-Based Authentication
```
User Login
    │
    ▼
Keycloak Authentication
    │
    ▼
Access Token (JWT) + Refresh Token (Encrypted)
    │
    ▼
Bearer Token in API Request
    │
    ▼
JWT Validation (Auth Service)
    │
    ▼
Token Validated ✓ → Service Access
```

#### Token Structure
```json
{
  "sub": "user-id",
  "iat": 1234567890,
  "exp": 1234654390,
  "type": "access_token",
  "iss": "digitalbank",
  "azp": "auth-service"
}
```

#### Authorization Strategy
- **Role-Based Access Control (RBAC)**: Service roles (admin, support, read-only)
- **Attribute-Based Access Control (ABAC)**: Resource attributes
- **Claim-Based Authorization**: Token claims validation

### Encryption & Key Management

#### Data Encryption at Rest
- **PostgreSQL**: TDE (Transparent Data Encryption)
- **Redis**: AES-256 encryption
- **Files**: AES-256 for sensitive files
- **Database Columns**: Column-level encryption

#### Data Encryption in Transit
- **TLS 1.3**: All service-to-service communication
- **mTLS**: Mutual TLS for service mesh
- **Certificate Rotation**: 90-day rotation

#### Key Management
- **HashiCorp Vault**: All encryption keys
- **Key Rotation**: Automatic periodic rotation
- **Key Access**: Role-based with audit logging

#### Card Data Security (PCI-DSS)
- **CVV/CVV2**: Encrypted with dedicated key
- **Card Number**: Tokenized, never stored plain
- **Cardholder Name**: Encrypted with dedicated key
- **Card Expiry**: Stored plain (PCI scope)

### Secrets Management

```
Infrastructure Layer
    │
    ▼
    HashiCorp Vault Server
    │
    └───┬───┬───┬───┬───┬───┬───┬───┬───┬───┬───┬───
        │   │   │   │   │   │   │   │   │   │   │
        ▼   ▼   ▼   ▼   ▼   ▼   ▼   ▼   ▼   ▼   ▼   ▼
      Kubernetes    Accounts     Payments    Cards    ...
        │        │        │        │        │        │
        ▼        ▼        ▼        ▼        ▼        ▼
     Secrets Vault    Secrets Vault    Secrets Vault  ...
```

### Compliance

#### PCI-DSS (Payment Card Industry)
- Requirement 1: Network segmentation
- Requirement 2: Encryption in transit/at rest
- Requirement 3: No standalone firewalls
- Requirement 4: Malware protection
- Requirement 5: Strict access control
- Requirement 6: ID access management
- Requirement 7: Cardholder data handling
- Requirement 8: Intrusion detection
- Requirement 9: Secure system config
- Requirement 10: Encryption key management
- Requirement 11: Security logging
- Requirement 12: Network monitoring

#### GDPR (General Data Protection)
- Right to access
- Right to rectification
- Right to erasure
- Right to data portability
- Data retention policies
- Consent management

### Security Logging

```yaml
# Security events logged:
- Authentication failures
- Token validation failures
- Unauthorized access attempts
- Permission denials
- Rate limit violations
- Suspicious patterns
- API abuse detection
```

### WAF Configuration
```yaml
# Protection against:
- SQL Injection
- XSS attacks
- Cross-site request forgery
- Command injection
- Path traversal
- Buffer overflows
- Brute force attacks
```

## Observability Strategy

### Metrics

#### Business Metrics
```
┌─────────────────────────────────────────────────────┐
│ Transaction Volume (rate, counters)                 │
│ Payment Success Rate                                │
│ Average Transaction Time                            │
│ Account Balance Distributions                        │
│ Daily Active Users                                  │
│ Revenue Metrics                                     │
└─────────────────────────────────────────────────────┘
```

#### Technical Metrics
```
┌─────────────────────────────────────────────────────┐
│ gRPC Metrics                                        │
│ ────────────────────────────────────              │
│ RequestLatency                                      │
│ RequestCount                                        │
│ RequestSize                                        │
│ RequestBytes                                        │
│ RequestDuration                                     │
│ RequestDurationHistogram                            │
│ RequestDurationSummary                              │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│ Database Metrics                                    │
│ ────────────────────────────────────              │
│ QueryDuration                                       │
│ QueryCount                                          │
│ ConnectionPoolUsage                                 │
│ ReplicationLag                                      │
│ DiskUsage                                           │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│ Infrastructure Metrics                              │
│ ────────────────────────────────────              │
│ CPU Usage                                           │
│ Memory Usage                                        │
│ Disk I/O                                            │
│ Network I/O                                         │
│ Container Restart Count                             │
└─────────────────────────────────────────────────────┘
```

### Distributed Tracing

#### Trace Structure
```
Trace: abc123
├── Request: xyz789 (API Gateway)
│   ├── Request: uvw456 (Auth Service)
│   ├── Request: rst123 (Accounts Service)
│   │   ├── Request: mno789 (Ledger Service)
│   │       ├── DB Query: qrs456
│   │       └── Event Publish: pqr123
│   │
│   ├── Request: jkl456 (Payments Service)
│   │   ├── Payment Initiation: hij789
│   │   ├── Card Validation: fgh123
│   │   └── Event Dispatch: def456
└── Request: ghi789 (Notification Service)
    └── Email Send: mno987
```

#### Key Traces
- Service spans with latency
- Database query traces
- External service calls
- Cache hit/miss events

### Logging Strategy

#### Log Structure
```json
{
  "timestamp": "2024-01-15T10:30:00.000Z",
  "trace_id": "abc123",
  "span_id": "xyz789",
  "service_name": "ledger-service",
  "service_version": "1.0.0",
  "level": "INFO",
  "message": "Transaction processed successfully",
  "user_id": "user-123",
  "request_id": "req-456",
  "duration_ms": 45,
  "host": "ledger-0.svc.cluster.local",
  "error": null,
  "request_id": "abc123"
}
```

#### Log Levels
- **DEBUG**: Debugging information (dev only)
- **INFO**: Service status, important events
- **WARN**: Warning conditions, recoverable errors
- **ERROR**: Errors requiring attention
- **FATAL**: Critical failures (system shutdown)

### Monitoring Dashboards

#### Executive Dashboard
- System Health Overview
- Key Business Metrics
- Alert Status
- Incident Timeline

#### Technical Dashboard - Gateway
- Request Rate
- Error Rate
- Latency Percentiles
- Rate Limiting
- Authentication Failures

#### Technical Dashboard - Services
- Service Uptime
- Request Rate
- Error Rate
- Response Latency
- Queue Depth

#### Technical Dashboard - Database
- Connection Pool Usage
- Query Duration
- Replication Lag
- Query Execution Time
- Lock Wait Time

#### Technical Dashboard - Infrastructure
- CPU Usage
- Memory Usage
- Disk I/O
- Network I/O

### Alert Configuration

#### Critical Alerts (P0)
- Database connection failures
- Service pod crashes
- Certificate expiration
- mTLS failures

#### High Priority Alerts (P1)
- High error rates (>5%)
- High latency (95th percentile > 2s)
- Low service availability
- Rate limit exhaustion
- High memory usage
- Disk space usage
- Replication lag (>30s)

#### Medium Priority Alerts (P2)
- Slow queries (>5s, 10 min window)
- High latency
- Queue depth (events)
- High CPU usage
- Connection pool exhaustion

#### Monitoring Rules
```yaml
# Prometheus alerting rules
groups:
  - name: critical
    rules:
      - alert: DatabaseConnectionFailures
        expr: rate(postgresql_connections_total{state="failed"}[5m]) > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: Database connection failures detected

      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) > 0.05
        for: 5m
        labels:
          severity: high
```

### Observability Pipeline

```
┌─────────────────────────────────────────────────────┐
│                    Services                         │
│  (generate traces, metrics, logs)                   │
└──────────────┬─────────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────┐
│                Agents                               │
│  - prometheus (metrics)                            │
│  - Jaeger Agent (traces)                          │
│  - Fluent Bit (logs)                               │
└──────────────┬─────────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────┐
│                 Storage                             │
│  - Prometheus (metrics)                            │
│  - Jaeger (traces)                                 │
│  - ELK Stack (logs)                                │
└──────────────┬─────────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────┐
│            Visualization                            │
│  - Grafana (dashboards)                           │
│  - Kibana (logs)                                  │
│  - Jaeger UI (tracing)                            │
│  - Prometheus UI (metrics)                        │
│  - PagerDuty (alerts)                            │
└─────────────────────────────────────────────────────┘
```

### Observability Best Practices

1. **Trace Context Propagation**: Pass trace_id across all service boundaries
2. **Correlation IDs**: Link user requests across services
3. **Structured Logging**: Use consistent log format across services
4. **Resource Labels**: Tag metrics with service, pod, etc.
5. **Distributed Tracing**: Full request path visibility
6. **Real User Monitoring (RUM)**: Track end-user experience
7. **A/B Testing Integration**: Track feature variants
8. **Error Grouping**: Cluster similar errors

## Technology Stack

### Core Technologies
- **Go 1.21**: Language for microservices
- **gRPC**: High-performance RPC framework
- **Protocol Buffers**: IDL definition language
- **PostgreSQL 15**: Primary database
- **Redis 7**: Caching and messaging

### Infrastructure
- **Kubernetes**: Container orchestration
- **Helm**: Package manager for Kubernetes
- **Docker**: Container runtime

### API Gateway
- **Kong**: API Gateway
- **OpenResty**: High-performance web server
- **Kong Ingress**: gRPC to HTTP translation

### Identity & Access
- **Keycloak**: Identity and Access Management
- **Vault**: Secrets management

### Observability
- **Prometheus**: Metrics collection
- **Grafana**: Dashboard visualization
- **Jaeger**: Distributed tracing
- **ELK Stack**: Log aggregation (Elasticsearch, Logstash, Kibana)

### DevOps
- **GitHub Actions**: CI/CD pipeline
- **ArgoCD**: GitOps
- **Flux**: GitOps controller

### External Services
- **Stripe**: Payment processing
- **Twilio**: SMS notifications
- **SendGrid**: Email notifications
- **Auth0**: Optional SSO

## Scalability

### Horizontal Scaling
- Services scale independently based on load
- Stateless service design
- Load balancing across instances

### Database Scaling
- Read replicas for read-heavy workloads
- Sharding by customer ID
- Archive old data to separate cluster

### Caching Strategy
- **Account Data**: Redis cache
- **Balance Data**: Redis cache
- **Session Data**: Redis cache
- **Rate Limits**: Redis cache

### Event-Driven Scaling
- Peak loads handled by message queue
- Services scale based on consumer demand
- Backpressure handling

## High Availability

### Multi-Region
- Active-active or active-passive
- Database replication across regions
- Cross-region failover

### Redundancy
- Multiple service instances
- Multi-AZ deployments
- Cross-zone load balancing

### Failover Strategy
- Health checks detect failures
- Automatic failover on primary failure
- Graceful degradation during partial outages

## Performance Optimization

### Database Optimization
- Connection pooling (PgBouncer)
- Query optimization
- Indexing strategy
- Query plan caching

### gRPC Optimization
- Connection pooling
- Message compression
- Streaming for large payloads
- Client-side caching

### Caching Optimization
- Smart cache invalidation
- Cache warming for hot paths
- TTL strategies

### Resource Management
- CPU limits
- Memory limits
- Request limits
- Load shedding during peak

---

## Appendix

### Service Health Check Script
```bash
#!/bin/bash
# health_check.sh

SERVICES=("auth ledger accounts payments cards transfers users notifications")
FAILURES=0

for service in "${SERVICES[@]}"; do
    if ! curl -sf "http://$service:8080/health"; then
        echo "FAILED: $service"
        ((FAILURES++))
    else
        echo "OK: $service"
    fi
done

if [ $FAILURES -gt 0 ]; then
    exit 1
fi

exit 0
```

### Disaster Recovery Plan

1. **Daily Snapshots**: Automated PostgreSQL snapshots
2. **Weekly Backups**: Point-in-time recovery
3. **Monthly DR Tests**: Failover testing
4. **RPO**: 1 hour (1h data loss acceptable)
5. **RTO**: 30 minutes

### Documentation Maintenance

- Architecture diagrams updated with each release
- API documentation versioned
- On-call rotations maintained
- Runbooks kept current with service updates

---

*Document Version: 1.0*
*Last Updated: 2024*