# Vigil - Email Fraud Detection System

Email discovery service for Google Workspace and Microsoft O365 that continuously polls for users and emails, stores metadata in PostgreSQL, and prepares emails for fraud analysis.

![Vigil Architecture](SPECS/vigil.png)

*Vigil processing emails from 1030 concurrent users*

## Architecture

### Key Components

- **Mock Server**: Simulates Google/Microsoft provider APIs (port 8080)
- **Discovery Service**: 
  - **User Discovery**: Polls provider every 1 minute, sends ADD_USER/REMOVE_USER messages
  - **Email Discovery**: Receives messages, creates channel generator per user (polls every 20 seconds)
  - **Fan-in Pattern**: Combines all user channels into one processing stream
  - **Processing Loop**: Fingerprint deduplication, DB storage, queue sending (stub)

- **PostgreSQL**: Stores users, emails (metadata only), and user_emails junction table

### Design Decisions

- **Metadata-only storage**: Stores email fingerprints and metadata, not full content (saves ~180TB/year at 10M emails/day)
- **Message-based decoupling**: User discovery and email discovery communicate via messages (enables separate pods later)
- **Incremental polling**: Tracks `last_email_received` per user for efficient polling
- **Channel generator pattern**: One goroutine per user for concurrent email discovery
- **Dynamic fan-in**: Automatically recreates when users are added/removed

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
./test.sh

# Inspect database
./inspect-db.sh 

# DIY
docker exec -it vigil-postgres psql -U vigil -d vigil -c "SELECT COUNT(*) FROM users;"
docker exec -it vigil-postgres psql -U vigil -d vigil -c "SELECT COUNT(*) FROM emails;"

# Add more users to test discovery
curl -X POST http://localhost:8080/admin/users/add?numUsers=10
```

## Database Schema

- **users**: `id`, `email`, `last_email_check`, `last_email_received`
- **emails**: `id` (message_id), `fingerprint` (SHA256), `received_at`
- **user_emails**: Junction table linking users to emails (many-to-many)

## Future Work

- [ ] Integrate actual message queue (Kafka/RabbitMQ/NATS) for analysis
- [ ] Build analysis workers for fraud detection
- [ ] Add rate limiting and retry logic
- [ ] Support multiple tenants
- [ ] Add metrics and monitoring
