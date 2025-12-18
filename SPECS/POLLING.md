# Polling Logic Deep-Dive

## The Core Challenge

We have a hierarchy: tenants → users → emails. At scale, we might have:
- Hundreds of tenants
- Thousands of users per tenant
- Millions of emails per day

We can't just spawn unlimited goroutines because:
1. API rate limits (Microsoft/Google will throttle us)
2. Database connection pool limits
3. Memory constraints
4. We need to track state (last check time per user)

## The Polling Flow

Imagine we wake up every polling interval (say, every 15 minutes). We need to:

1. **Discover tenants to process** - Query PostgreSQL for active tenants
2. **For each tenant, discover users** - Call `GetMicrosoftUsers(tenantID)` or `GetGoogleUsers(tenantID)`
3. **For each user, fetch new emails** - Call `GetMicrosoftEmails(userID, receivedAfter, orderBy)` where `receivedAfter` is the last check time we stored

The tricky part: we need to coordinate thousands of these operations without overwhelming APIs or the database.

## A Practical Approach

**Stage 1: Tenant Discovery (Sequential or Small Pool)**
- Query DB for active tenants (maybe 100-500 tenants)
- This is fast, no API calls yet
- Could use a small worker pool (10-20 goroutines) if we want to parallelize tenant-level operations

**Stage 2: User Discovery (Rate-Limited Pool)**
- For each tenant, we need to call the provider API to get users
- This is where rate limits matter
- Use a **rate-limited worker pool** - maybe 50-100 goroutines max, with a rate limiter that respects provider limits
- Each goroutine: fetch users for one tenant, update DB with discovered users, then move on

**Stage 3: Email Polling (The Big One)**
- For each user, poll for emails
- This is where we might need thousands of goroutines, but we can't just spawn them all
- Use a **bounded worker pool** with rate limiting

## The Worker Pool Pattern

Here's how I'd structure it:

```
Main Scheduler (goroutine)
  ↓
  Discovers tenants from DB
  ↓
  Enqueues tenant IDs to tenant queue
  ↓
Tenant Worker Pool (50-100 workers, rate-limited)
  ↓
  For each tenant:
    - Fetch users via API
    - Update DB with user discovery
    - Enqueue user IDs to user queue
  ↓
User Worker Pool (1000-5000 workers, rate-limited)
  ↓
  For each user:
    - Query DB for last_check_time
    - Fetch emails via API (receivedAfter = last_check_time)
    - Enqueue email metadata to analysis queue
    - Update DB with new last_check_time
```

## Key Implementation Details

**Rate Limiting Strategy:**
- Use a token bucket or sliding window rate limiter
- Separate limiters per provider (Microsoft vs Google might have different limits)
- Maybe 100-200 API calls/second per provider (need to test/configure)
- Workers block when tokens aren't available

**Database Access:**
- Use connection pooling (pgxpool in Go)
- Batch updates where possible (e.g., bulk insert discovered users)
- Read last_check_time in batches, not one-by-one
- Use transactions for atomic updates

**Error Handling:**
- If API call fails (429 rate limit, timeout), back off and retry
- Use exponential backoff
- Dead-letter queue for users that consistently fail
- Don't let one failing tenant/user block others

**State Management:**
- Before polling a user, read `last_check_time` from DB
- After successful poll, update `last_check_time` atomically
- Use `receivedAfter` parameter in API call
- Handle race conditions (what if two workers poll same user? Use DB locks or idempotency)

## A Concrete Example

Let's say we have:
- 200 tenants
- 10,000 users total
- Polling interval: 15 minutes

**Option A: Sequential (too slow)**
- 200 tenant API calls + 10,000 user API calls = 10,200 calls
- If each call takes 100ms = 17 minutes (exceeds our 15min window!)

**Option B: Unbounded goroutines (breaks rate limits)**
- Spawn 10,000 goroutines → API throttles us → everything fails

**Option C: Bounded pool with rate limiting (the sweet spot)**
- 100 tenant workers (process 200 tenants in ~2 seconds)
- 500 user workers, rate-limited to 150 API calls/second
- 10,000 users / 150 calls/sec = ~67 seconds to poll all users
- Well within 15-minute window, respects rate limits

## The Go Code Structure (Conceptually)

```go
// Pseudo-code structure
func main() {
    // Start scheduler
    go scheduler.Run()
    
    // Start worker pools
    tenantPool := NewWorkerPool(100, rateLimiter)
    userPool := NewWorkerPool(500, rateLimiter)
    
    tenantPool.Start()
    userPool.Start()
    
    // Scheduler enqueues work
    // Workers process concurrently
    // Rate limiters prevent API overload
}
```

## Edge Cases to Consider

1. **New users discovered mid-poll**: If we discover a new user while polling, what's their `receivedAfter`? Maybe last 30 days, or start from now?
2. **User deleted**: Handle gracefully, mark as inactive in DB
3. **Tenant deactivated**: Stop polling immediately
4. **API changes**: Provider adds pagination, changes response format
5. **Clock skew**: What if provider's `received_at` doesn't match our clock?
6. **Duplicate emails**: Use `message_id` for deduplication

## The Queue Integration

The email polling workers don't just store to DB - they also enqueue to the analysis queue (Kafka/RabbitMQ). This is where we decouple:
- Polling workers: fast, just fetch and enqueue
- Analysis workers: slower, do fraud detection
- If analysis is slow, polling keeps going (emails queue up)

