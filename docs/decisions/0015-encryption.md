# ADR-015: Encryption Strategy

**Status:** Accepted  
**Date:** 2026-11-16  
**Author:** Platform Team

## Context

The digital bank platform must encrypt sensitive data to comply with:
- PCI-DSS for card data
- GDPR for personal data
- Local financial regulations
- General data protection best practices

### Data Classification

| Category | Examples | Protection Level |
|----------|----------|------------------|
| PCI | Card numbers, CVV, cardholder names | Highest (tokenization + encryption) |
| PII | Names, addresses, SSN/ID numbers | High (encryption) |
| Financial | Account numbers, balances, transactions | High (encryption) |
| System | Logs, configuration, health checks | Medium |

## Decision

**We will implement defense-in-depth encryption with multiple layers: application-layer field encryption, database-level transparent data encryption, and transport-layer TLS.**

### Encryption Layers

#### 1. Transport Layer (TLS)

**All External Traffic:**
- TLS 1.3 with TLS 1.2 fallback
- ECDSA P-256 certificates
- HSTS headers
- Certificate pinning for mobile clients

**Internal Traffic:**
- mTLS (via service mesh, see ADR-016)
- Certificate rotation every 90 days
- Ciphers: TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256

#### 2. Database Layer

**Transparent Data Encryption (TDE):**
- Database-level encryption via cloud provider KMS
- Master key rotation every 180 days
- Auto-encryption for backups

**Column-Level Encryption:**
- Sensitive columns encrypted at write-time
- Decryption transparent on read (within same service)
- Encrypted columns cannot be queried

**PCI-DSS Specific:**
- Card numbers: Encrypted via separate card key
- CVV: Encrypted immediately (never stored)
- Cardholder name: Tokenized (not stored)

#### 3. Application Layer

**Field-Level Encryption:**
- Sensitive fields encrypted before database write
- Key management via dedicated key service
- Separate keys per environment (dev/staging/prod)

**Encryption at Rest (Files):**
- Upload/download encrypted files via client-side encryption
- Encrypted file storage in object storage with server-side encryption

### Key Management

**Architecture:**
```
Application Layer Keys (App)
  ↓ (wrapped)
Key Management Service (KMS)
  ↓ (wrapped)
Hardware Security Module (HSM) / Cloud KMS
  ↓
Cloud Provider (AWS KMS, GCP KMS, Azure Key Vault)
```

**Key Lifecycle:**
- Generation: At app startup or on first use
- Storage: Encrypted in secure memory (dev), hardware-backed (prod)
- Usage: Request via KMS API for encryption/decryption
- Rotation: Automatic (cloud KMS) or controlled (dedicated KMS)
- Backup: Encrypted key backups
- Revocation: Rekey affected data

**Key Separation:**
- Separate keys per data category (PCI, PII, financial)
- Separate keys per environment
- Separate keys per service (if no KMS)
- Separate keys per tenant/customer

### PCI-DSS Compliance

**Requirement 3 - Cardholder Data:**
- Card numbers encrypted with strong cryptography (AES-256)
- Encrypted PANs encrypted with master card key
- CVV used once, never stored

**Requirement 4 - Data Minimization:**
- Tokenize card data for payments
- Store only token, not PAN

**Requirement 6 - Key Management:**
- Dedicated cryptographers manage keys
- Strict key access controls
- Regular key rotation

### Implementation Details

#### Cloud Provider Options

**AWS:**
- AWS KMS for key management
- AWS Secrets Manager for secrets
- AWS CloudHSM for dedicated HSM

**GCP:**
- GCP KMS for key management
- CMEK (Customer Managed Encryption Keys)

**Azure:**
- Azure Key Vault for key management
- Managed HSM for HSM

#### Database Implementation

**PostgreSQL:**
- TDE via cloud provider
- Column-level encryption (pgcrypto extension)
- Encrypted backup/restore

**MongoDB:**
- TDE at collection level
- Application-level encryption for specific fields

#### Tokenization Service

**Card Tokenization:**
- Map PAN to token (reversible via token service)
- Store token in database (encrypted)
- Card data retained in PCI-scoped system

**Token Format:**
```
12345678901234568 (PAN) → Tkn:5432:6789:1234:56789 (Token)
```

## Consequences

### Pros
- Comprehensive data protection
- Compliance with PCI-DSS, GDPR, and financial regulations
- Protection against database compromises
- Defense in depth

### Cons
- Complexity in key management
- Performance overhead (especially field-level encryption)
- Decryption limits in databases
- Migration complexity for data rekeying

### Mitigations
- Use cloud KMS for automatic key management
- Field-level encryption with asynchronous processing
- Caching encrypted values in application memory
- Performance testing with realistic data volumes

### Security Notes

- Never log sensitive data (even encrypted)
- Never transmit keys over unencrypted channels
- Regular security audits of encryption implementation
- Vulnerability scanning for cryptographic weaknesses

## Related Decisions

- ADR-014: Zero-Trust Security - Related for transport encryption
- ADR-016: Mutual TLS - Detailed inter-service encryption
- ADR-023: Key Management Service - Future elaboration
- ADR-024: Tokenization Strategy - Future elaboration

## References

- PCI-DSS: https://www.pcisecuritystandards.org/
- GDPR: https://gdpr.eu/
- NIST SP 800-111: Encrypting Sensitive Data
- TLS 1.3: https://www.ietf.org/rfc/rfc8446