#!/bin/bash

# Basic E2E Test Script for Universal Chat Interface
# Tests the complete flow using curl commands

set -e  # Exit on error

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
RESET='\033[0m'

# Configuration
BASE_URL="${BASE_URL:-http://localhost}"
SESSION_ID="${SESSION_ID:-test-session-$(date +%s)}"
USER_ID="${USER_ID:-test-user}"
SSE_TIMEOUT=30

# Test counters
PASSED=0
FAILED=0

# Helper functions
print_header() {
    echo ""
    echo -e "${BOLD}${BLUE}$1${RESET}"
    echo -e "${BLUE}$(printf '─%.0s' {1..60})${RESET}"
}

print_success() {
    echo -e "${GREEN}✓${RESET} $1"
    ((PASSED++))
}

print_failure() {
    echo -e "${RED}✗${RESET} $1"
    echo -e "  ${RED}Error: $2${RESET}"
    ((FAILED++))
}

print_info() {
    echo -e "${BLUE}  ℹ${RESET} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${RESET} $1"
}

# Test 1: Check if services are running
test_services() {
    print_header "Checking Required Services"

    # Check Redis
    if nc -z localhost 6379 2>/dev/null; then
        print_success "Redis (port 6379)"
    else
        print_failure "Redis" "Not responding on port 6379"
    fi

    # Check NATS
    if nc -z localhost 4222 2>/dev/null; then
        print_success "NATS (port 4222)"
    else
        print_failure "NATS" "Not responding on port 4222"
    fi

    # Check PostgreSQL
    if nc -z localhost 5432 2>/dev/null; then
        print_success "PostgreSQL (port 5432)"
    else
        print_failure "PostgreSQL" "Not responding on port 5432"
    fi

    # Check Nginx
    if nc -z localhost 80 2>/dev/null; then
        print_success "Nginx (port 80)"
    else
        print_failure "Nginx" "Not responding on port 80"
    fi
}

# Test 2: Health checks
test_health_checks() {
    print_header "Testing Health Checks"

    # Load balancer health
    if curl -s -f "${BASE_URL}/health" > /dev/null; then
        print_success "Load Balancer Health"
        HEALTH=$(curl -s "${BASE_URL}/health")
        POD_ID=$(echo "$HEALTH" | jq -r '.pod_id // "unknown"')
        CONNECTIONS=$(echo "$HEALTH" | jq -r '.active_connections // 0')
        print_info "Routed to: $POD_ID, Active connections: $CONNECTIONS"
    else
        print_failure "Load Balancer Health" "Health check failed"
    fi

    # Individual pods
    for port in 8081 8082 8083; do
        POD_NUM=$((port - 8080))
        if curl -s -f "${BASE_URL}:${port}/health" > /dev/null; then
            print_success "Backend Pod $POD_NUM Health (port $port)"
        else
            print_failure "Backend Pod $POD_NUM Health" "Not responding on port $port"
        fi
    done
}

# Test 3: Chat message submission
test_chat_submission() {
    print_header "Testing Chat Message Submission"

    RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/chat" \
        -H "Content-Type: application/json" \
        -d "{
            \"session_id\": \"${SESSION_ID}\",
            \"user_id\": \"${USER_ID}\",
            \"message\": \"Hello! This is a test message from the bash script.\"
        }")

    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" == "202" ]; then
        print_success "Chat Message Submitted (202 Accepted)"
        MESSAGE_ID=$(echo "$BODY" | jq -r '.message_id // "unknown"')
        CORRELATION_ID=$(echo "$BODY" | jq -r '.correlation_id // "unknown"')
        print_info "Message ID: $MESSAGE_ID"
        print_info "Correlation ID: $CORRELATION_ID"
    else
        print_failure "Chat Message Submission" "Expected 202, got $HTTP_CODE"
        echo "$BODY"
    fi
}

# Test 4: SSE streaming
test_sse_streaming() {
    print_header "Testing SSE Streaming"

    print_info "Opening SSE connection and submitting message..."

    # Create a named pipe for communication
    SSE_PIPE="/tmp/sse_test_$$"
    mkfifo "$SSE_PIPE"

    # Start SSE connection in background
    (
        curl -s -N "${BASE_URL}/api/sse?session_id=${SESSION_ID}&user_id=${USER_ID}" > "$SSE_PIPE" &
        SSE_PID=$!

        # Kill SSE connection after timeout
        sleep $SSE_TIMEOUT
        kill $SSE_PID 2>/dev/null || true
    ) &
    BG_PID=$!

    # Wait a moment for SSE to establish
    sleep 2

    # Submit chat message
    curl -s -X POST "${BASE_URL}/api/chat" \
        -H "Content-Type: application/json" \
        -d "{
            \"session_id\": \"${SESSION_ID}\",
            \"user_id\": \"${USER_ID}\",
            \"message\": \"Test message for SSE streaming\"
        }" > /dev/null

    # Read SSE events
    CHUNK_COUNT=0
    CONNECTED=false
    MESSAGE_COMPLETE=false
    TIMEOUT=10

    print_info "Waiting for SSE events (timeout: ${TIMEOUT}s)..."

    while IFS= read -r -t $TIMEOUT line; do
        if [[ $line =~ ^event:\ connected ]]; then
            CONNECTED=true
        elif [[ $line =~ ^data:.*chunk_id ]]; then
            ((CHUNK_COUNT++))
            # Extract chunk preview
            CHUNK_TEXT=$(echo "$line" | sed 's/^data: //' | jq -r '.chunk // ""' 2>/dev/null)
            CHUNK_ID=$(echo "$line" | sed 's/^data: //' | jq -r '.chunk_id // ""' 2>/dev/null)
            print_info "Chunk $CHUNK_ID: ${CHUNK_TEXT:0:30}..."
        elif [[ $line =~ ^data:.*message_complete ]]; then
            MESSAGE_COMPLETE=true
            break
        fi
    done < "$SSE_PIPE"

    # Cleanup
    rm -f "$SSE_PIPE"
    kill $BG_PID 2>/dev/null || true

    # Validate results
    if [ "$CONNECTED" = true ]; then
        print_success "SSE Connection Established"
    else
        print_failure "SSE Connection" "Connection event not received"
    fi

    if [ $CHUNK_COUNT -gt 0 ]; then
        print_success "Received $CHUNK_COUNT chunks"
    else
        print_failure "Chunk Reception" "No chunks received"
    fi

    if [ "$MESSAGE_COMPLETE" = true ]; then
        print_success "Message completed successfully"
    else
        print_warning "Message completion event not received (may have timed out)"
    fi
}

# Test 5: Concurrent connections
test_concurrent_connections() {
    print_header "Testing Concurrent Connections"

    print_info "Testing if multiple SSE connections work for same session..."

    # This is a simplified test - just verify we can open multiple connections
    for i in 1 2 3; do
        if timeout 2 curl -s -N "${BASE_URL}/api/sse?session_id=${SESSION_ID}-concurrent&user_id=${USER_ID}" > /dev/null 2>&1; then
            print_success "Concurrent connection $i established"
        else
            print_warning "Concurrent connection $i timed out (may be normal)"
        fi
        sleep 0.5
    done
}

# Main test execution
main() {
    echo ""
    echo -e "${BOLD}========================================${RESET}"
    echo -e "${BOLD}Universal Chat Interface - E2E Tests${RESET}"
    echo -e "${BOLD}========================================${RESET}"
    echo -e "Base URL: $BASE_URL"
    echo -e "Session ID: $SESSION_ID"
    echo -e "User ID: $USER_ID"
    echo -e "Time: $(date '+%Y-%m-%d %H:%M:%S')"

    # Run tests
    test_services
    test_health_checks
    test_chat_submission
    test_sse_streaming
    # test_concurrent_connections  # Optional - can be slow

    # Print summary
    echo ""
    echo -e "${BOLD}========================================${RESET}"
    echo -e "${BOLD}Test Summary${RESET}"
    echo -e "${BOLD}========================================${RESET}"
    echo -e "  Passed: ${GREEN}$PASSED${RESET}"
    echo -e "  Failed: ${RED}$FAILED${RESET}"
    echo -e "${BOLD}========================================${RESET}"
    echo ""

    # Exit with appropriate code
    if [ $FAILED -eq 0 ]; then
        echo -e "${GREEN}${BOLD}All tests passed!${RESET}"
        exit 0
    else
        echo -e "${RED}${BOLD}Some tests failed!${RESET}"
        exit 1
    fi
}

# Check for required commands
for cmd in curl jq nc; do
    if ! command -v $cmd &> /dev/null; then
        echo -e "${RED}Error: Required command '$cmd' not found${RESET}"
        echo -e "Please install: sudo apt-get install $cmd"
        exit 1
    fi
done

# Run main
main
