# Vigil Project - Full Conversation Transcript

## Initial Request

**User**: Technical test requirements for Go Senior Backend Engineer position - Email Security tool to flag emails with potential financial fraud attempts. Requirements include:
- Use excalidraw or similar for architecture
- Build service in Go
- Use PostgreSQL
- Dockerize the service
- Tests
- Mock functions for Microsoft/Google APIs provided

**User**: "We are working to work together on this assignment. To start, please suggest cool github repo name for that project"

**AI**: Suggested several names including `fraudguard-email`, `email-sentinel`, `phishguard`, etc.

**User**: "I prefer 'vigil', not email-vigil. If it was a real product, that is how one would call it"

**AI**: Acknowledged preference for "vigil" as product name.

## Brainstorming Rules

**User**: 
- Expect concise answers always
- First, give general thought about the problem

**AI**: Provided general thoughts on architecture (event-driven, queue-based, microservices), scalability (10M emails/day), reliability (idempotency, retries), fraud detection (multi-stage pipeline), storage considerations.

**User**: "another rule: bullet point list like this one have zero value for reason about the problem. We are discovering the problem here. What I need from is more like if you were telling me a story. For instance, start with how we would practically use these APIs"

## Understanding the Problem

**AI**: Explained practical usage of APIs:
- Start with tenant, call GetMicrosoftUsers/GetGoogleUsers to get all users
- For each user, call GetMicrosoftEmails/GetGoogleEmails with receivedAfter for incremental polling
- Questions about when to run (continuously vs schedule), pagination, rate limits
- Need to track last processed timestamp per user
- orderBy helps with incremental processing
- Rate limits are constraint - need rate-limited queue
- Parallelization needed but within API limits

**User**: "what is a tenant?"

**AI**: Explained tenant = client organization using Google Workspace or Microsoft O365. Each tenant has tenantID and multiple users (employees). We provide fraud detection service to multiple tenants.

**User**: "OK. So, to summarize:
- there is a set of tenants that we need to process
- we need to continously poll the Google/MS service for the entire set of tenants to detect new users
- for each user, we need to continously poll for new emails
do I miss something?"

**AI**: Added:
- Initial discovery: need starting point when first processing tenant/user (last 30 days or start from now)
- What happens after fetching: emails need to be analyzed for fraud - separate step from retrieval
- "Continuously" likely means periodic polling (e.g., every 15 minutes), not truly continuous

**User**: "it does"

## Database Design

**User**: "how do you we use a postgres db?"

**AI**: Explained PostgreSQL usage:
- Track state: which tenants to process, which users discovered, when last checked
- Store fetched emails with message_id, received_at to avoid duplicates
- Track tenant metadata (provider, configuration)
- Store analysis results (fraud scores, flags, reasons)
- Database is source of truth for what's been processed, enables resume after failures

**User**: "I want to challenge the idea of storing the entire email content. The data is originally in MS and Google database and we are disussing about replicating that data. - if we don't store the entire email content, can we not still reprocess that email later by querying the provider services?"

**AI**: Yes, can store only metadata (message_id, received_at, sender, subject, snippet). When need full content, query provider API using message_id or received_at. Trade-off: reprocessing slower (API call vs DB query) and depends on provider API availability.

**User**: "> query the provider API using message_id or received_at. there is no API using message_id. we can query using received_at at that will suffice. Do you see a problem with that?"

**AI**: Problem: if only use received_at, can't target specific email - would fetch all emails around that timestamp and filter client-side. Solution: still store message_id as unique identifier (metadata, not full content). When reprocessing, fetch by received_at and match by message_id.

**User**: "you are making a good point: When reprocessing, fetch by received_at and match by message_id. I AGREE. the main reason I am pushing back on the idea of storing the entire email content is that we need to scale to 10 millions emails / day. storing email content would be too much right?"

**AI**: Confirmed: At 10M emails/day, storing full content (~50KB avg) = ~500GB/day = ~180TB/year. Metadata-only (~1-2KB) = ~10-20GB/day, much more manageable. Fetch full content from provider API only when needed.

## Critical Questions

**User**: "if you had to ask only 1 question to the person that wrote the requirement, what would it be?"

**AI**: "How do we get the list of tenants to process?" (Is there registry/API, database table, config file?)

**User**: "I believe the tenants are companies that let us fetch their user email and are paying for the vigil service"

**AI**: So we maintain registry of paying tenants in PostgreSQL: tenant_id, provider, subscription status, credentials. Process only active, paid tenants. Implies tenant management: onboarding, deactivation, credential rotation.

**User**: "that question can be handled in a trivial. There are many unknows in the requirements. We can just assume we have a list of tenants. Do you have a more relevant question to ask?"

**AI**: "What happens after we flag an email as potential fraud?" (notify, store, expose API, trigger actions?) - affects whether we need notification systems, webhooks, or just storage.

**User**: "okay. That is relevant. another question ? keep considering that there is only 1 question we can ask. I will decide what is the most relevant but consider that the other ones, we will have to assume an answer"

**AI**: "How do we handle API authentication/credentials per tenant?" (OAuth tokens, API keys, storage, rotation, token expiration/refresh?)

**User**: "that is secondary honestly"

**AI**: "What's the expected latency from email receipt to fraud detection?" (real-time minutes vs batch hours/days?) - determines polling frequency, affects rate limit handling, shapes architecture.

**User**: "I personally think this is the most important question. Do you see why?"

**AI**: Yes - latency requirement drives entire polling strategy. If need detection within minutes, can't poll every hour - need frequent polling, more API calls, rate limit pressure, forces prioritization. If can tolerate hours/days, can spread polling, reduce API pressure, simpler scheduled processing. Also affects prioritization: process all tenants equally or prioritize high-value ones?

**User**: "how many emails per seconds to account for 10M emails/day?"

**AI**: ~116 emails/second average (10M / 86,400 seconds). Peak likely 2-3x higher during business hours = ~300-400 emails/second at peak.

**User**: "That forces prioritization, rate-limit handling, and possibly event-driven processing. what do you mean by event-driven processing?"

**AI**: Clarified: was imprecise. Since we're polling Microsoft/Google APIs (no webhooks), can't be event-driven at ingestion. Meant: once fetch emails, process asynchronously through queue rather than synchronously. Fetch → enqueue → workers analyze in parallel. Decouples fetching from analysis, allows independent scaling. But fetching itself is still polling, not event-driven. "Event-driven" part is only internal pipeline after we have emails.

**User**: "so we have a queue to send emails for analysis. What happens at the other end of the queue?"

**AI**: Workers consume from queue:
1. Dequeue email (metadata)
2. Fetch full content from provider API if needed
3. Extract features (sender patterns, content analysis, timing anomalies, etc.)
4. Run fraud detection (rule-based, ML models, heuristics)
5. Store results in PostgreSQL (fraud score, flags, reasons)
6. Optionally trigger notifications/actions if fraud detected

This is where fraud detection logic runs. Queue decouples ingestion from analysis, allows independent worker scaling, handles bursts.

**User**: "what do you suggest?"

**AI**: Multi-stage pipeline:
- Stage 1: Quick rules - fast filters for obvious patterns (sender domain mismatch, suspicious keywords, unknown sender requesting money)
- Stage 2: ML/pattern matching - for emails passing Stage 1, extract features (sender reputation, email history, content similarity, timing anomalies, relationship graph)
- Stage 3: Scoring - output fraud score (0-100) rather than binary. Store all features and scores for model improvement.

Key is storing features, not just results. Over time, can train models on what actually gets flagged.

Pipeline: workers extract features, run rules, then ML models, store everything, flag high-confidence cases for immediate alerting.

**User**: "I don't have much experience with ML. I was considering to use some of the HuggingFace model"

**AI**: Good approach. Pre-trained models can help with text classification, sentiment analysis, named entity recognition, language models for detecting unusual phrasing/impersonation. Simple approach: extract email text, run through HuggingFace model, get score, combine with rule-based features for final fraud score. Pipeline stays same: workers fetch content, extract features, run rules, call HuggingFace model, combine scores, store results.

## Documentation

**User**: "let's continue with the architecture. I will like to save our thoughts to a README file now"

**AI**: Created README.md with architectural decisions, processing pipeline, fraud detection approach, scalability considerations, reliability requirements.

**User**: "- Shapes whether this is event-driven or scheduled batch processing. I recall we said we can not use event-driven. We have to use the provider API"

**AI**: Fixed README - we're using scheduled batch processing (polling provider APIs). Latency requirement just determines polling frequency, not the processing model.

**User**: "save this conversation as a specification file. I will need to resume it from within another Cursor window"

**AI**: Created SPEC.md with comprehensive specification including context, architectural decisions, rationale, critical unknowns, assumptions, scalability considerations, reliability requirements, technical requirements, next steps.

**User**: "save this entire conversation to a file"

**AI**: Created this CONVERSATION.md file with full transcript.

