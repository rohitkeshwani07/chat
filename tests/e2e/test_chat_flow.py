#!/usr/bin/env python3
"""
End-to-End Test for Universal Chat Interface

Tests the complete message flow:
1. Start SSE connection
2. Submit chat message
3. Receive streamed response chunks
4. Verify chunk ordering and completeness
5. Validate metadata in final chunk
"""

import asyncio
import json
import sys
import time
from datetime import datetime
from typing import List, Dict
import argparse

import aiohttp
import requests

# ANSI color codes for pretty output
GREEN = '\033[92m'
RED = '\033[91m'
YELLOW = '\033[93m'
BLUE = '\033[94m'
RESET = '\033[0m'
BOLD = '\033[1m'


class TestResult:
    def __init__(self):
        self.passed = 0
        self.failed = 0
        self.warnings = 0
        self.errors = []

    def pass_test(self, name: str):
        self.passed += 1
        print(f"{GREEN}✓{RESET} {name}")

    def fail_test(self, name: str, error: str):
        self.failed += 1
        self.errors.append(f"{name}: {error}")
        print(f"{RED}✗{RESET} {name}")
        print(f"  {RED}Error: {error}{RESET}")

    def warn(self, message: str):
        self.warnings += 1
        print(f"{YELLOW}⚠{RESET} {message}")

    def print_summary(self):
        print(f"\n{BOLD}{'='*60}{RESET}")
        print(f"{BOLD}Test Summary{RESET}")
        print(f"{'='*60}")
        print(f"  Passed: {GREEN}{self.passed}{RESET}")
        print(f"  Failed: {RED}{self.failed}{RESET}")
        print(f"  Warnings: {YELLOW}{self.warnings}{RESET}")

        if self.errors:
            print(f"\n{BOLD}Errors:{RESET}")
            for error in self.errors:
                print(f"  {RED}•{RESET} {error}")

        print(f"{'='*60}\n")

        return self.failed == 0


async def test_sse_connection(base_url: str, session_id: str, user_id: str, result: TestResult):
    """Test SSE connection and message streaming"""

    print(f"\n{BOLD}{BLUE}Testing SSE Connection & Message Flow{RESET}")
    print(f"{'─'*60}")

    # Track received chunks
    received_chunks = []
    connection_established = False
    message_complete = False

    async with aiohttp.ClientSession() as session:
        # Open SSE connection
        sse_url = f"{base_url}/api/sse?session_id={session_id}&user_id={user_id}"

        try:
            async with session.get(sse_url, timeout=aiohttp.ClientTimeout(total=30)) as sse_response:
                if sse_response.status != 200:
                    result.fail_test("SSE Connection", f"Status {sse_response.status}")
                    return

                result.pass_test("SSE Connection Established")

                # Start reading SSE events
                async def read_sse():
                    nonlocal connection_established, message_complete

                    async for line in sse_response.content:
                        line = line.decode('utf-8').strip()

                        if not line:
                            continue

                        if line.startswith('event:'):
                            event_type = line.split(':', 1)[1].strip()
                        elif line.startswith('data:'):
                            data = line.split(':', 1)[1].strip()
                            try:
                                event_data = json.loads(data)

                                if not connection_established:
                                    if 'connection_id' in event_data:
                                        connection_established = True
                                        print(f"{BLUE}  ℹ{RESET} Connection ID: {event_data.get('connection_id')}")

                                if 'chunk_id' in event_data:
                                    received_chunks.append(event_data)
                                    chunk_preview = event_data.get('chunk', '')[:30]
                                    print(f"{BLUE}  →{RESET} Chunk {event_data['chunk_id']}: {chunk_preview}...")

                                    if event_data.get('is_final'):
                                        message_complete = True
                                        return

                                if event_data.get('message_id') and 'token_count' in event_data:
                                    print(f"{BLUE}  ℹ{RESET} Message complete: {event_data.get('token_count')} tokens")
                                    message_complete = True
                                    return

                            except json.JSONDecodeError as e:
                                result.warn(f"Failed to parse SSE data: {e}")

                # Submit chat message after a short delay (simulate real usage)
                async def submit_message():
                    await asyncio.sleep(1)  # Wait for SSE to be ready

                    chat_url = f"{base_url}/api/chat"
                    payload = {
                        "session_id": session_id,
                        "user_id": user_id,
                        "message": "Hello! This is a test message."
                    }

                    try:
                        async with session.post(chat_url, json=payload) as response:
                            if response.status == 202:
                                result.pass_test("Chat Message Submitted")
                                response_data = await response.json()
                                print(f"{BLUE}  ℹ{RESET} Message ID: {response_data.get('message_id')}")
                            else:
                                result.fail_test("Chat Message Submission", f"Status {response.status}")
                    except Exception as e:
                        result.fail_test("Chat Message Submission", str(e))

                # Run both tasks concurrently
                await asyncio.gather(
                    read_sse(),
                    submit_message()
                )

        except asyncio.TimeoutError:
            result.fail_test("SSE Streaming", "Timeout waiting for response")
            return
        except Exception as e:
            result.fail_test("SSE Streaming", str(e))
            return

    # Validate received chunks
    print(f"\n{BOLD}Validating Response Chunks{RESET}")
    print(f"{'─'*60}")

    if not received_chunks:
        result.fail_test("Chunk Reception", "No chunks received")
        return

    result.pass_test(f"Received {len(received_chunks)} chunks")

    # Check chunk ordering
    chunk_ids = [chunk['chunk_id'] for chunk in received_chunks]
    expected_ids = list(range(len(received_chunks)))

    if chunk_ids == expected_ids:
        result.pass_test("Chunks in correct order")
    else:
        result.fail_test("Chunk Ordering", f"Expected {expected_ids}, got {chunk_ids}")

    # Check for final chunk
    final_chunk = received_chunks[-1] if received_chunks else None
    if final_chunk and final_chunk.get('is_final'):
        result.pass_test("Final chunk marked correctly")

        # Check metadata
        if 'metadata' in final_chunk:
            result.pass_test("Metadata present in final chunk")
            metadata = final_chunk['metadata']

            if 'tokens_used' in metadata:
                result.pass_test(f"Token count: {metadata['tokens_used']}")
            else:
                result.warn("Token count missing in metadata")
        else:
            result.warn("Metadata missing in final chunk")
    else:
        result.fail_test("Final Chunk", "is_final flag not set correctly")

    # Reconstruct full message
    full_message = ''.join([chunk.get('chunk', '') for chunk in received_chunks])
    if full_message:
        result.pass_test("Message reconstruction successful")
        print(f"{BLUE}  ℹ{RESET} Full message ({len(full_message)} chars):")
        print(f"    {full_message[:100]}...")
    else:
        result.fail_test("Message Reconstruction", "Empty message")


def test_health_check(base_url: str, result: TestResult):
    """Test health check endpoints"""

    print(f"\n{BOLD}{BLUE}Testing Health Checks{RESET}")
    print(f"{'─'*60}")

    endpoints = [
        (f"{base_url}/health", "Load Balancer Health"),
        (f"{base_url}:8081/health", "Backend Pod 1 Health"),
        (f"{base_url}:8082/health", "Backend Pod 2 Health"),
        (f"{base_url}:8083/health", "Backend Pod 3 Health"),
    ]

    for url, name in endpoints:
        try:
            response = requests.get(url, timeout=5)
            if response.status_code == 200:
                data = response.json()
                result.pass_test(name)
                pod_id = data.get('pod_id', 'N/A')
                connections = data.get('active_connections', 0)
                print(f"{BLUE}  ℹ{RESET} Pod: {pod_id}, Connections: {connections}")
            else:
                result.fail_test(name, f"Status {response.status_code}")
        except requests.exceptions.ConnectionError:
            result.fail_test(name, "Connection refused - service may be down")
        except Exception as e:
            result.fail_test(name, str(e))


def test_services_running(result: TestResult):
    """Check if required services are running"""

    print(f"\n{BOLD}{BLUE}Checking Required Services{RESET}")
    print(f"{'─'*60}")

    services = [
        ("localhost", 6379, "Redis"),
        ("localhost", 4222, "NATS"),
        ("localhost", 5432, "PostgreSQL"),
        ("localhost", 80, "Nginx (Load Balancer)"),
    ]

    import socket

    for host, port, name in services:
        try:
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(2)
            result_code = sock.connect_ex((host, port))
            sock.close()

            if result_code == 0:
                result.pass_test(f"{name} (port {port})")
            else:
                result.fail_test(f"{name} Service", f"Not responding on port {port}")
        except Exception as e:
            result.fail_test(f"{name} Service", str(e))


async def run_tests(base_url: str, session_id: str, user_id: str):
    """Run all tests"""

    print(f"\n{BOLD}{'='*60}{RESET}")
    print(f"{BOLD}Universal Chat Interface - E2E Tests{RESET}")
    print(f"{'='*60}")
    print(f"Base URL: {base_url}")
    print(f"Session ID: {session_id}")
    print(f"User ID: {user_id}")
    print(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")

    result = TestResult()

    # Test 1: Check services are running
    test_services_running(result)

    # Test 2: Health checks
    test_health_check(base_url, result)

    # Test 3: SSE and message flow
    await test_sse_connection(base_url, session_id, user_id, result)

    # Print summary
    success = result.print_summary()

    return 0 if success else 1


def main():
    parser = argparse.ArgumentParser(description='E2E tests for Universal Chat Interface')
    parser.add_argument('--url', default='http://localhost', help='Base URL (default: http://localhost)')
    parser.add_argument('--session', default='test-session', help='Session ID (default: test-session)')
    parser.add_argument('--user', default='test-user', help='User ID (default: test-user)')

    args = parser.parse_args()

    try:
        exit_code = asyncio.run(run_tests(args.url, args.session, args.user))
        sys.exit(exit_code)
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Tests interrupted by user{RESET}")
        sys.exit(1)
    except Exception as e:
        print(f"\n{RED}Fatal error: {e}{RESET}")
        sys.exit(1)


if __name__ == "__main__":
    main()
