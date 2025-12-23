# Mock Server

Mock API server that simulates Google Workspace provider APIs for testing and development.

## Running Locally

```bash
# From project root
go run services/mock-server/main.go
```

Or build and run:
```bash
go build -o mock-server services/mock-server/main.go
./mock-server
```

The server will start on port 8080 by default. Set `PORT` environment variable to change it.

## Docker

Build the image:
```bash
docker build -t vigil-mock-server -f services/mock-server/Dockerfile .
```

Run the container:
```bash
docker run -p 8080:8080 vigil-mock-server
```

Or with custom port:
```bash
docker run -p 9090:8080 -e PORT=8080 vigil-mock-server
```

## API Endpoints

### Provider Endpoints

- `GET /health` - Health check
- `GET /google/users/:tenantId` - Get users for a tenant
- `GET /google/emails/:userId?receivedAfter=...&orderBy=...` - Get emails for a user

### Admin Endpoints

- `POST /admin/users/add` - Add new users to the mock server

  **Request:**
  - JSON body (optional): `{"numUsers": 20}`
  - Query parameter (optional): `?numUsers=20`
  - Defaults to 1 if not specified

  **Response:**
  ```json
  {
    "added": 20,
    "total": 1020,
    "message": "Added 20 user(s). Total users: 1020"
  }
  ```

  **Example:**
  ```bash
  # Using JSON body
  curl -X POST http://localhost:8080/admin/users/add \
    -H "Content-Type: application/json" \
    -d '{"numUsers": 20}'

  # Using query parameter
  curl -X POST "http://localhost:8080/admin/users/add?numUsers=20"
  ```

