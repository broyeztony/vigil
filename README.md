# Vigil - Email Fraud Detection System

Email discovery service for Google Workspace and Microsoft O365 that continuously polls for users and emails, stores metadata in PostgreSQL, and prepares emails for fraud analysis.

## Karl Migration

Vigil has been fully migrated to the Karl programming language:  
[broyeztony/vigil_in_karl](https://github.com/broyeztony/vigil_in_karl)

![Vigil Demo](SPECS/vigil_5000.png)

*Vigil graceful shutdown after processing ~270K emails from 5000 concurrent users for ~15m*

## Architecture

![Vigil Architecture](SPECS/architecture.png)

### Key Components

- **Mock Server**: Simulates Google/Microsoft provider APIs (port 8080)
- **Discovery Service**: 
  - **User Discovery**: Polls provider every 1 minute, sends ADD_USER/REMOVE_USER messages
  - **Email Discovery**: Receives messages, creates channel generator per user (polls every 30 seconds)
  - **Fan-in Pattern**: Combines all user channels into one processing stream
  - **Processing Loop**: Fingerprint deduplication, DB storage, queue sending (stub)

- **PostgreSQL**: Stores users, emails (metadata only), and user_emails junction table

### Design Decisions

- **Zero Copy Principle**: Only stores email metadata (fingerprint, received_at), not full content. Full email content is fetched from provider API only when needed for analysis. This saves ~180TB/year at 10M emails/day and ensures GDPR compliance.
- **Channel Generator Pattern**: Each user = 1 goroutine + 1 buffered channel. The goroutine polls the provider API every 30 seconds and streams emails to its dedicated channel.
- **Fan-in Pattern**: A central collection point combines all user channels into a single processing stream. The fan-in is dynamically recreated when users are added/removed.
- **Message-based Decoupling**: User discovery and email discovery communicate via messages (`ADD_USER`/`REMOVE_USER`), enabling separate pods/namespaces later.
- **Incremental Polling**: Tracks `last_email_received` per user for efficient incremental polling (only fetches new emails).
- **Kubernetes-Ready**: Designed for Kubernetes with 1 tenant = 1 namespace. Each namespace runs a dedicated discovery service pod managing all users for that tenant. A Kubernetes operator could be implemented for tenant provisioning and lifecycle management.

## Quick Start

```bash
# Start all services (PostgreSQL, mock-server, discovery-service)
docker-compose up -d --build

# Watch logs
docker-compose logs -f discovery-service

# Stop all services
docker-compose down
```

## Local Development

### 1. Start PostgreSQL

```bash
docker-compose up -d postgres
```

### 2. Setup Database

```bash
go run ./services/discovery-service/cmd/discovery setup \
  --database.url "postgres://vigil:vigil@localhost:5432/vigil?sslmode=disable"
```

### 3. Start Mock Server

```bash
# Terminal 1
go run ./services/mock-server/main.go
```

Or with Docker:
```bash
docker-compose up -d mock-server
```

### 4. Run Discovery Service

```bash
# Terminal 2
go run ./services/discovery-service/cmd/discovery run \
  --database.url "postgres://vigil:vigil@localhost:5432/vigil?sslmode=disable" \
  --tenant_id "00000000-0000-0000-0000-000000000001" \
  --provider.api_url "http://localhost:8080" \
  --provider.type "google"
```

## API Endpoints

### Mock Server (Port 8080)

- `GET /health` - Health check
- `GET /google/users/:tenantId` - Get users for a tenant
- `GET /google/emails/:userId?receivedAfter=...&orderBy=...` - Get emails for a user
- `POST /admin/users/add?numUsers=20` - Add users to mock server (for testing)

**Example:**
```bash
# Add 20 users
curl -X POST http://localhost:8080/admin/users/add?numUsers=20

# Get users
curl http://localhost:8080/google/users/00000000-0000-0000-0000-000000000001
```

## How It Works

1. **User Discovery** (every 1 minute):
   - Polls provider API for users
   - Upserts users to database
   - Sends `ADD_USER`/`REMOVE_USER` messages

2. **Email Discovery** (receives messages):
   - Starts email polling goroutine per user (30s interval)
   - Uses `last_email_received` for incremental polling
   - Sends emails to fan-in channel

3. **Processing**:
   - Fingerprint deduplication (SHA256 of body)
   - Stores metadata in PostgreSQL
   - Sends unique emails to analysis queue (stub implementation)
   - Updates user timestamps

## Testing

```bash
# Run test
./scripts/test.sh

# Inspect database
./scripts/inspect-db.sh 

# Test graceful shutdown
./scripts/test-graceful-shutdown.sh

# Watch database changes
./scripts/watch-db.sh

# Manual testing
docker exec -it vigil-postgres psql -U vigil -d vigil -c "SELECT COUNT(*) FROM users;"
docker exec -it vigil-postgres psql -U vigil -d vigil -c "SELECT COUNT(*) FROM emails;"
curl -X POST http://localhost:8080/admin/users/add?numUsers=10
```

## Database Schema

- **users**: `id`, `email`, `last_email_check`, `last_email_received`
- **emails**: `id` (message_id), `fingerprint` (SHA256), `received_at`
- **user_emails**: Junction table linking users to emails (many-to-many)

## Implementation Notes

- **Code Generation**: Some portions of the code were AI-generated, but based on prototyping code and technical decisions that can be fully justified. The core architecture and concurrency patterns were designed and prototyped manually.
- **Email Analysis/ML**: The email analysis and machine learning components are not fully documented yet. This is currently being worked on in preparation for a potential in-person interview discussion.

## Future Work

- [ ] Integrate actual message queue (Kafka/RabbitMQ/NATS) for analysis
- [ ] Build analysis workers for fraud detection (ML models, feature extraction)
- [ ] Add rate limiting and retry logic (handle 429 responses with exponential backoff)
- [ ] Implement Kubernetes operator for tenant provisioning and lifecycle management
- [ ] Add comprehensive metrics and monitoring (Prometheus, Grafana)
- [ ] Support multiple tenants with proper isolation
