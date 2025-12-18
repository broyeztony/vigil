# Vigil - Email Fraud Detection System

## Overview

Vigil is an email security tool that flags emails with potential financial fraud attempts (fake president, fake supplier, etc.) for clients using Google Workspace and Microsoft O365.

## Problem Statement

We need to continuously poll a set of tenants (paying clients) to:
1. Discover new users for each tenant
2. Poll each user for new emails
3. Analyze emails for fraud indicators
4. Scale to handle 10 million emails per day (~116 emails/second average, ~300-400/second peak)

## Architecture Decisions

### Data Storage Strategy

**Metadata-only storage in PostgreSQL**: We store only email metadata (message_id, received_at, sender, subject, snippet) rather than full email content. This keeps storage manageable at scale (~1-2KB per email vs ~50KB for full content).

For reprocessing, we fetch full content from the provider API using `received_at` and match by `message_id`. This avoids replicating massive amounts of data while still allowing us to reprocess emails when needed.

**State tracking**: PostgreSQL stores:
- Tenant registry (active tenants, provider type, credentials)
- User discovery state (which users we've found, last check time)
- Email metadata (what we've fetched, when)
- Analysis results (fraud scores, flags, features)

### Processing Pipeline

**Two-stage architecture**:

1. **Ingestion Service**: Continuously polls Microsoft/Google APIs
   - Fetches users for each tenant
   - Fetches new emails for each user (using `receivedAfter` for incremental polling)
   - Enqueues emails to a message queue for analysis

2. **Analysis Workers**: Consume from queue
   - Fetch full email content from provider API when needed
   - Extract features
   - Run fraud detection (rules + ML models)
   - Store results in PostgreSQL

**Queue-based decoupling**: A message queue (Kafka/RabbitMQ) decouples ingestion from analysis, allowing:
- Independent scaling of ingestion vs analysis
- Handling bursts in email volume
- Resilience to failures (emails stay in queue if workers fail)

### Fraud Detection Pipeline

**Multi-stage approach**:

1. **Quick Rules**: Fast filtering for obvious patterns
   - Sender domain mismatches
   - Suspicious keywords
   - Unknown sender requesting money

2. **ML/Analysis**: For emails passing initial rules
   - HuggingFace models for text classification
   - Feature extraction (sender patterns, timing, relationships)
   - Combined scoring

3. **Scoring**: Output fraud score (0-100) rather than binary
   - Store all features and scores for model improvement
   - Enable manual review workflows

## Key Questions

**Critical unknown**: What's the expected latency from email receipt to fraud detection?
- Real-time (minutes) vs batch (hours/days) determines polling frequency
- Affects rate limit handling and prioritization strategies
- Since we're polling provider APIs (no webhooks), this is always scheduled batch processing - the question is how frequently we schedule it

## Scalability Considerations

At 10M emails/day:
- Horizontal scaling via worker pools
- Batch processing where possible
- Partitioning by tenant/user
- Caching for user/tenant lookups
- Rate limit management for provider APIs

## Reliability

- Idempotent processing (message_id deduplication)
- Dead-letter queues for failures
- Retries with exponential backoff
- Checkpointing for resume after failures

