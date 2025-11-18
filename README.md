# Universal Chat Interface

A flexible chat interface designed to work with any AI chatbot. This project provides a universal frontend and backend architecture that can integrate with various AI models and chatbot services.

## Project Structure

```
├── architecture/     # Architecture documentation and diagrams
├── backend/         # Golang backend service
├── frontend/        # React-based chat interface
└── README.md        # This file
```

## Overview

This project aims to create a modular chat interface that can be easily configured to work with different AI chatbot backends. The React frontend provides a clean, user-friendly chat experience, while the Golang backend handles API integrations and message routing to various AI services.

## Features

- Universal chat interface compatible with multiple AI chatbot services
- React-based responsive frontend
- Golang backend for high-performance message handling
- Modular architecture for easy integration with different AI providers

## Quick Start with Docker

The easiest way to run the entire system is using Docker Compose:

```bash
# Clone the repository
git clone https://github.com/rohitkeshwani07/chat.git
cd chat

# (Optional) Copy and customize environment variables
cp .env.example .env

# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop all services
docker-compose down
```

This will start:
- **3 Backend Pods** (ports 8081, 8082, 8083)
- **Nginx Load Balancer** (port 80)
- **PostgreSQL** (port 5432)
- **Redis** (port 6379)
- **NATS** (port 4222)

### Access the Services

- **API (via Load Balancer)**: http://localhost
- **Health Check**: http://localhost/health
- **Backend Pod 1**: http://localhost:8081
- **Backend Pod 2**: http://localhost:8082
- **Backend Pod 3**: http://localhost:8083
- **NATS Monitoring**: http://localhost/nats/

### Test the API

```bash
# Submit a chat message
curl -X POST http://localhost/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "test-session",
    "user_id": "test-user",
    "message": "Hello, AI!"
  }'

# Connect to SSE stream (in another terminal)
curl -N http://localhost/api/sse?session_id=test-session&user_id=test-user
```

## Architecture

This project implements a highly available, distributed chat system:

- **Multiple Backend Pods**: Stateless Go services behind load balancer
- **NATS Messaging**: Asynchronous workflow execution and response routing
- **Redis Session Registry**: Tracks active SSE connections across pods
- **Chunk Buffering**: Handles out-of-order message delivery with in-memory buffering
- **SSE Streaming**: Real-time response streaming to clients

See `/architecture` for detailed documentation.

## Development Setup

### Prerequisites

- Docker and Docker Compose
- Go 1.21+ (for local development)
- Node.js 18+ (for frontend development)

### Run Backend Locally (without Docker)

```bash
# Start dependencies
docker-compose up -d postgres redis nats

# Run backend
cd backend
go run cmd/server/main.go
```

See `/backend/README.md` for more details.

## Project Components

### Backend (`/backend`)
Golang microservice with:
- HTTP API (REST + SSE)
- NATS message handling
- Redis session tracking
- In-memory chunk buffering
- Out-of-order message handling

### Architecture (`/architecture`)
Complete system design documentation:
- Backend architecture
- Database schemas
- NATS messaging patterns
- Chunk buffering and ordering

### Frontend (`/frontend`)
React-based chat interface (coming soon)

## Configuration

Environment variables can be set in `.env` file:

```bash
# Database
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=chat

# Redis
REDIS_PASSWORD=

# See .env.example for all options
```

## Monitoring

Check service health:

```bash
# Overall health (via load balancer)
curl http://localhost/health

# Individual pod health
curl http://localhost:8081/health
curl http://localhost:8082/health
curl http://localhost:8083/health
```

## Scaling

Scale backend pods:

```bash
# Scale to 5 pods
docker-compose up -d --scale backend-1=5

# Or edit docker-compose.yml to add more pod definitions
```

## Troubleshooting

### Services won't start
```bash
# Check logs
docker-compose logs

# Restart all services
docker-compose restart
```

### Port conflicts
Edit `docker-compose.yml` to change port mappings if you have conflicts.

### Clear all data
```bash
# Stop and remove containers, networks, and volumes
docker-compose down -v
```

## Next Steps

- [ ] Implement database migrations
- [ ] Add workflow service for AI integration
- [ ] Build React frontend
- [ ] Add authentication
- [ ] Implement rate limiting
- [ ] Add monitoring (Prometheus/Grafana)
- [ ] Production Kubernetes manifests
