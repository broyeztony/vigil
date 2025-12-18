# Vigil - Technical Specification

## Context

This document captures the architectural decisions and discussions for building Vigil, an email fraud detection system for Google Workspace and Microsoft O365 clients.

```
# Go Senior Backend Engineer — Technical test

## Introduction

We want to build an **Email Security** tool to flag emails with potential financial fraud attempts (fake president, fake supplier, etc.) that are received by our clients. We focus on clients that are using Google Workspace and Microsoft O365 so users and emails can be retrieved through relevant APIs. 

The goal of this test is to:
- design the entire architecture of a system that is reliable and scalable.
- code the service retrieving the emails. 
- reflect on how to flag such emails: (1) enumerate feature ideas in the readme, (2) think about the pipeline. 

## Requirements

### Must have

- Use excalidraw or similar tool to write down the architecture
- Build the service in Go
- Use PostreSQL
- Dockerize the service
- Tests

### Liberties 

You can assume the following functions (or very similar variants) exist (and mock them).

func GetMicrosoftUsers(tenantID uuid.UUID) ([]MicrosoftUser, error) 
func GetMicrosoftEmails(userID uuid.UUID, receivedAfter time.Time, orderBy string) ([]MicrosoftEmail, error)
func GetGoogleUsers(tenantID uuid.UUID) ([]GoogleUser, error)
func GetGoogleEmails(userID uuid.UUID, receivedAfter time.Time, orderBy string) ([]GoogleEmail, error)

## What we will look for
- Do you a clear plan to make the design evolve towards a system supporting 10 million emails / day?
- How your system will cope with failures and spikes in the workload?
- Do you have interesting ideas, can discuss them with energy and can execute efficiently?
- Are you able to switch from macro view to micro details?
- Is the discussion constructive?
```

## Problem Statement

Build a system that:
- Continuously polls tenants (paying clients) to discover users
- Polls each user for new emails
- Analyzes emails for financial fraud indicators (fake president, fake supplier, etc.)
- Scales to 10 million emails per day (~116 emails/second average, ~300-400/second peak)

## Key Architectural Decisions

### 1. Data Storage Strategy

**Decision**: Store only email metadata in PostgreSQL, not full email content.

**Rationale**: 
- At 10M emails/day, storing full content (~50KB avg) = ~500GB/day = ~180TB/year
- Metadata-only (~1-2KB avg) = ~10-20GB/day, much more manageable
- Full content can be fetched from provider API when needed for reprocessing

**Implementation**:
- Store: message_id, received_at, sender, subject, snippet
- For reprocessing: fetch by received_at, match by message_id

**PostgreSQL stores**:
- Tenant registry (tenant_id, provider type, credentials, subscription status)
- User discovery state (user_id, tenant_id, last_email_check timestamp)
- Email metadata (message_id, received_at, sender, subject, snippet, user_id)
- Analysis results (email_id, fraud_score, flags, features, analysis_timestamp)

### 2. Processing Architecture

**Decision**: Two-stage architecture with queue-based decoupling.

**Stage 1 - Ingestion Service**:
- Continuously polls Microsoft/Google APIs
- Fetches users for each tenant using `GetMicrosoftUsers(tenantID)` / `GetGoogleUsers(tenantID)`
- For each user, fetches new emails using `GetMicrosoftEmails(userID, receivedAfter, orderBy)` / `GetGoogleEmails(...)`
- Uses `receivedAfter` for incremental polling (tracks last processed timestamp per user)
- Enqueues email metadata to message queue for analysis

**Stage 2 - Analysis Workers**:
- Consume email metadata from queue
- Fetch full email content from provider API when needed
- Extract features
- Run fraud detection pipeline
- Store results in PostgreSQL

**Queue choice**: Kafka or RabbitMQ for decoupling ingestion from analysis.

**Benefits**:
- Independent scaling of ingestion vs analysis
- Handle bursts in email volume
- Resilience to failures (emails stay in queue if workers fail)

### 3. Fraud Detection Pipeline

**Decision**: Multi-stage approach with rule-based filtering + ML analysis.

**Stage 1 - Quick Rules** (fast filtering):
- Sender domain doesn't match display name
- Suspicious keywords ("urgent wire transfer", "CEO request")
- Links to unknown domains
- Unknown sender requesting money

**Stage 2 - ML/Analysis** (for emails passing Stage 1):
- Use HuggingFace models for text classification
- Extract features: sender reputation, email history, content similarity, timing anomalies, relationship graph
- Combined scoring

**Stage 3 - Scoring**:
- Output fraud score (0-100) rather than binary
- Store all features and scores for model improvement
- Enable manual review workflows

### 4. Polling Strategy

**Decision**: Scheduled batch processing (not event-driven).

**Rationale**: 
- Provider APIs don't provide webhooks/events
- Must poll Microsoft/Google APIs directly
- Always scheduled batch processing, frequency TBD based on latency requirements

**Implementation considerations**:
- Track last check time per user in PostgreSQL
- Use `receivedAfter` parameter for incremental polling
- Handle rate limits gracefully (429 responses)
- Prioritize tenants/users based on business needs

## Critical Unknowns

### Latency Requirement (Most Important Question)

**Question**: What's the expected latency from email receipt to fraud detection?

**Impact**:
- Real-time (minutes) → frequent polling (every 5-15 min) → higher API pressure
- Batch (hours/days) → less frequent polling → more manageable rate limits

**Determines**:
- Polling frequency
- Rate limit handling strategy
- Prioritization needs
- System complexity

### Post-Detection Actions

**Question**: What happens after we flag an email as potential fraud?

**Options**:
- Store for review
- Notify someone (email, webhook, API)
- Trigger automated actions
- Expose via API for client access

**Impact**: Determines if we need notification systems, webhooks, or just storage.

## Assumptions

1. **Tenant Registry**: We have a list of active tenants (paying clients) stored in PostgreSQL
2. **API Functions**: The following functions exist (or similar variants):
   ```go
   func GetMicrosoftUsers(tenantID uuid.UUID) ([]MicrosoftUser, error)
   func GetMicrosoftEmails(userID uuid.UUID, receivedAfter time.Time, orderBy string) ([]MicrosoftEmail, error)
   func GetGoogleUsers(tenantID uuid.UUID) ([]GoogleUser, error)
   func GetGoogleEmails(userID uuid.UUID, receivedAfter time.Time, orderBy string) ([]GoogleEmail, error)
   ```
3. **Initial Discovery**: When first processing a tenant/user, we need to decide initial `receivedAfter` timestamp (e.g., last 30 days or start from now)
4. **API Authentication**: Credentials/tokens per tenant are handled (storage, rotation, refresh) - considered secondary concern

## Scalability Considerations

At 10M emails/day:
- Horizontal scaling via worker pools
- Batch processing where possible
- Partitioning by tenant/user
- Caching for user/tenant lookups
- Rate limit management for provider APIs

## Reliability Requirements

- Idempotent processing (message_id deduplication)
- Dead-letter queues for failures
- Retries with exponential backoff
- Checkpointing for resume after failures
- Handle API failures gracefully (429s, timeouts, etc.)

## Technical Requirements

- Language: Go
- Database: PostgreSQL
- Containerization: Docker
- Tests: Required
- Architecture diagram: Excalidraw or similar

## Next Steps

1. Design detailed architecture diagram
2. Implement ingestion service (Go)
3. Set up PostgreSQL schema
4. Implement queue integration
5. Build analysis workers with fraud detection pipeline
6. Dockerize the service
7. Write tests
8. Document fraud detection feature ideas in README

