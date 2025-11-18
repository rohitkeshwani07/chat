# End-to-End Tests

Comprehensive test suite for the Universal Chat Interface system.

## Overview

These tests verify the complete message flow through the distributed architecture:

1. **Service Health** - Verify all services are running
2. **HTTP API** - Test chat message submission
3. **SSE Streaming** - Validate real-time response streaming
4. **Chunk Ordering** - Verify out-of-order chunk handling
5. **Multi-Pod Routing** - Test message routing across pods
6. **Metadata Validation** - Check response metadata completeness

## Test Scripts

### 1. Python Test Suite (Recommended)

**File**: `e2e/test_chat_flow.py`

Comprehensive async test with detailed validation:
- Proper SSE event handling
- Chunk ordering verification
- Metadata validation
- Colored output with detailed results
- JSON response parsing

**Prerequisites**:
```bash
# Install dependencies
pip install -r e2e/requirements.txt

# Or using venv
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
pip install -r e2e/requirements.txt
```

**Usage**:
```bash
# Basic usage (defaults to http://localhost)
python e2e/test_chat_flow.py

# Custom configuration
python e2e/test_chat_flow.py \
  --url http://localhost \
  --session my-test-session \
  --user test-user-123

# With different base URL
python e2e/test_chat_flow.py --url http://192.168.1.100
```

**Output Example**:
```
============================================================
Universal Chat Interface - E2E Tests
============================================================
Base URL: http://localhost
Session ID: test-session
User ID: test-user
Time: 2025-11-18 12:00:00

Checking Required Services
────────────────────────────────────────────────────────────
✓ Redis (port 6379)
✓ NATS (port 4222)
✓ PostgreSQL (port 5432)
✓ Nginx (port 80)

Testing Health Checks
────────────────────────────────────────────────────────────
✓ Load Balancer Health
  ℹ Pod: pod-2, Connections: 0
✓ Backend Pod 1 Health
✓ Backend Pod 2 Health
✓ Backend Pod 3 Health

Testing SSE Connection & Message Flow
────────────────────────────────────────────────────────────
✓ SSE Connection Established
  ℹ Connection ID: abc-123-xyz
✓ Chat Message Submitted
  ℹ Message ID: msg-456
  → Chunk 0: I received your message: 'H...
  → Chunk 1: ello! This is a test me...
  ...
  ℹ Message complete: 42 tokens

Validating Response Chunks
────────────────────────────────────────────────────────────
✓ Received 15 chunks
✓ Chunks in correct order
✓ Final chunk marked correctly
✓ Metadata present in final chunk
✓ Token count: 42
✓ Message reconstruction successful

============================================================
Test Summary
============================================================
  Passed: 18
  Failed: 0
  Warnings: 0
============================================================
```

### 2. Bash Test Script (Quick Testing)

**File**: `e2e/test_basic.sh`

Fast shell-based test using curl:
- No dependencies (except curl, jq, nc)
- Quick smoke tests
- Good for CI/CD pipelines
- Simpler output

**Prerequisites**:
```bash
# Install required tools (Ubuntu/Debian)
sudo apt-get install curl jq netcat-openbsd

# macOS
brew install curl jq netcat
```

**Usage**:
```bash
# Make executable (if not already)
chmod +x e2e/test_basic.sh

# Run tests
./e2e/test_basic.sh

# With custom configuration
BASE_URL=http://localhost SESSION_ID=my-session ./e2e/test_basic.sh

# With environment variables
export BASE_URL=http://192.168.1.100
export SESSION_ID=test-$(date +%s)
export USER_ID=test-user
./e2e/test_basic.sh
```

**Output Example**:
```
========================================
Universal Chat Interface - E2E Tests
========================================
Base URL: http://localhost
Session ID: test-session-1234567890
User ID: test-user
Time: 2025-11-18 12:00:00

Checking Required Services
────────────────────────────────────────────────────────────
✓ Redis (port 6379)
✓ NATS (port 4222)
✓ PostgreSQL (port 5432)
✓ Nginx (port 80)

Testing Health Checks
────────────────────────────────────────────────────────────
✓ Load Balancer Health
  ℹ Routed to: pod-1, Active connections: 0
✓ Backend Pod 1 Health (port 8081)
✓ Backend Pod 2 Health (port 8082)
✓ Backend Pod 3 Health (port 8083)

Testing Chat Message Submission
────────────────────────────────────────────────────────────
✓ Chat Message Submitted (202 Accepted)
  ℹ Message ID: msg-abc123
  ℹ Correlation ID: corr-xyz789

Testing SSE Streaming
────────────────────────────────────────────────────────────
  ℹ Opening SSE connection and submitting message...
  ℹ Waiting for SSE events (timeout: 10s)...
✓ SSE Connection Established
  ℹ Chunk 0: I received your...
  ℹ Chunk 1: message: 'Test message...
  ...
✓ Received 15 chunks
✓ Message completed successfully

========================================
Test Summary
========================================
  Passed: 14
  Failed: 0
========================================

All tests passed!
```

## Running Before Starting System

If you want to test that services will start properly:

```bash
# Start services
docker-compose up -d

# Wait for services to be ready (30 seconds)
sleep 30

# Run tests
python e2e/test_chat_flow.py
```

## Running in CI/CD

### GitHub Actions Example

```yaml
name: E2E Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3

      - name: Start services
        run: docker-compose up -d

      - name: Wait for services
        run: sleep 30

      - name: Install Python dependencies
        run: |
          pip install -r tests/e2e/requirements.txt

      - name: Run E2E tests
        run: |
          python tests/e2e/test_chat_flow.py

      - name: Stop services
        if: always()
        run: docker-compose down -v
```

### GitLab CI Example

```yaml
e2e-tests:
  image: python:3.11
  services:
    - docker:dind
  before_script:
    - docker-compose up -d
    - sleep 30
  script:
    - pip install -r tests/e2e/requirements.txt
    - python tests/e2e/test_chat_flow.py
  after_script:
    - docker-compose down -v
```

## Manual Testing Workflow

### Step-by-Step Manual Test

1. **Start all services**:
   ```bash
   docker-compose up -d
   ```

2. **Check services are healthy**:
   ```bash
   docker-compose ps
   docker-compose logs -f
   ```

3. **Run health checks**:
   ```bash
   curl http://localhost/health
   curl http://localhost:8081/health
   curl http://localhost:8082/health
   curl http://localhost:8083/health
   ```

4. **Open SSE connection** (Terminal 1):
   ```bash
   curl -N "http://localhost/api/sse?session_id=manual-test&user_id=tester"
   ```

5. **Submit message** (Terminal 2):
   ```bash
   curl -X POST http://localhost/api/chat \
     -H "Content-Type: application/json" \
     -d '{
       "session_id": "manual-test",
       "user_id": "tester",
       "message": "Hello from manual test!"
     }'
   ```

6. **Observe chunks streaming** in Terminal 1:
   ```
   event: connected
   data: {"connection_id":"...","session_id":"manual-test"}

   event: chunk
   data: {"session_id":"manual-test","chunk_id":0,"chunk":"I received your ","is_final":false}

   event: chunk
   data: {"session_id":"manual-test","chunk_id":1,"chunk":"message: 'Hello from ","is_final":false}
   ...
   ```

## Troubleshooting

### Tests Fail with "Connection Refused"

**Problem**: Services not running or not ready

**Solution**:
```bash
# Check if services are running
docker-compose ps

# Check logs
docker-compose logs

# Restart services
docker-compose restart

# Or rebuild and restart
docker-compose down -v
docker-compose up -d --build
```

### SSE Connection Times Out

**Problem**: No response chunks received

**Solutions**:

1. **Check workflow service is running**:
   ```bash
   docker-compose logs workflow
   ```

2. **Check NATS connectivity**:
   ```bash
   # Access NATS monitoring
   curl http://localhost/nats/subsz
   ```

3. **Check Redis session registry**:
   ```bash
   # Connect to Redis
   docker-compose exec redis redis-cli

   # Check for session connections
   KEYS session:connections:*
   ```

4. **Check backend logs**:
   ```bash
   docker-compose logs backend-1 backend-2 backend-3
   ```

### Chunks Arrive Out of Order

**Expected behavior** - The buffer manager should handle this!

**Verification**:
```bash
# Check backend logs for buffering activity
docker-compose logs backend-1 | grep -i buffer

# The test should still pass even with out-of-order chunks
python e2e/test_chat_flow.py
```

### "No chunks received"

**Causes**:
1. Workflow service not publishing
2. NATS routing issue
3. Backend not subscribed to correct subject

**Debug**:
```bash
# Check workflow service logs
docker-compose logs workflow

# Check NATS subjects
curl http://localhost/nats/subsz

# Manual test NATS
docker-compose exec backend-1 sh
# Inside container, check NATS connection
```

## Test Configuration

### Environment Variables

Both test scripts support environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://localhost` | Base URL for API |
| `SESSION_ID` | `test-session` | Session ID for tests |
| `USER_ID` | `test-user` | User ID for tests |
| `SSE_TIMEOUT` | `30` (bash) | SSE connection timeout |

### Custom Test Scenarios

You can create custom test scenarios by modifying session IDs:

```bash
# Test with specific session
python e2e/test_chat_flow.py --session prod-migration-test

# Test with timestamp for uniqueness
SESSION_ID="test-$(date +%s)" ./e2e/test_basic.sh

# Test multiple sessions in parallel
for i in {1..5}; do
  python e2e/test_chat_flow.py --session "parallel-test-$i" &
done
wait
```

## Expected Results

### Successful Test Run

- ✅ All services responding on correct ports
- ✅ Health checks return 200 OK
- ✅ Chat submission returns 202 Accepted
- ✅ SSE connection establishes successfully
- ✅ Chunks received in correct order (0, 1, 2, ...)
- ✅ Final chunk has `is_final: true`
- ✅ Metadata present with token count
- ✅ Full message reconstructs correctly

### Performance Expectations

- SSE connection: < 1 second
- Message submission: < 100ms
- First chunk arrival: < 2 seconds
- Complete response: < 10 seconds (for demo)

## Next Steps

- [ ] Add load testing (k6 or Locust)
- [ ] Add stress tests for concurrent connections
- [ ] Test failure scenarios (service outages)
- [ ] Add integration tests for database persistence
- [ ] Create performance benchmarks
- [ ] Add monitoring/metrics validation
