# Discovery Service

Service that discovers users and emails for tenants using the mock provider API. Uses the channel generator pattern for concurrent email discovery.

## Features

- Discovers users for a tenant (periodically)
- Discovers emails for all users (using channel generator + fan-in pattern)
- Stores users and emails in PostgreSQL
- Supports many-to-many relationship between users and emails (via junction table)

## Database Schema

- **users**: Stores user information with `last_email_check` and `last_email_received` timestamps
- **emails**: Stores email content (one record per unique message_id)
- **user_emails**: Junction table linking users to emails (supports emails sent to multiple users)

## Setup

1. **Start PostgreSQL:**
```bash
docker run -d --name vigil-postgres \
  -e POSTGRES_USER=vigil \
  -e POSTGRES_PASSWORD=vigil \
  -e POSTGRES_DB=vigil \
  -p 5432:5432 \
  postgres:15
```

2. **Start the mock server:**
```bash
cd services/mock-server
docker-compose up -d
```

3. **Run the discovery service:**
```bash
go run services/discovery-service/cmd/discovery/main.go run \
  --database.url "postgres://vigil:vigil@localhost:5432/vigil?sslmode=disable" \
  --tenant_id "00000000-0000-0000-0000-000000000001" \
  --provider.api_url "http://localhost:8080"
```

## Configuration

Create a `config.yaml` file:

```yaml
database:
  url: "postgres://vigil:vigil@localhost:5432/vigil?sslmode=disable"

tenant_id: "00000000-0000-0000-0000-000000000001"

provider:
  api_url: "http://localhost:8080"

polling:
  interval: 30  # seconds - fixed polling interval for all users
```

Or use command-line flags (see `discovery run --help`).

## How It Works

1. **User Discovery**: Runs every 1 minute, fetches users from provider API, stores/updates in database
2. **Email Discovery**: 
   - Creates a channel generator for each user
   - Each generator polls emails every 30 seconds
   - Fan-in pattern combines all user channels into one
   - Emails are stored in database and linked to users via junction table

## Architecture

- Uses channel generator pattern for concurrent email discovery
- Fan-in pattern to combine multiple user channels into a single processing stream
- Database stores emails once (deduplicated by fingerprint), links to multiple users via `user_emails` table

