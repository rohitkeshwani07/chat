#!/usr/bin/env python3
"""
Demo LangGraph Workflow Service

Simulates an AI workflow that:
1. Subscribes to NATS workflow execution requests
2. Generates streaming response chunks (simulating AI)
3. Queries Redis session registry to find active pods
4. Publishes chunks to appropriate pod subjects
"""

import asyncio
import json
import logging
import os
import time
from datetime import datetime
from typing import List, Dict

import nats
from nats.aio.client import Client as NATSClient
import redis.asyncio as redis

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='[%(asctime)s] [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger(__name__)


class WorkflowService:
    """Demo workflow service that simulates AI responses"""

    def __init__(self):
        self.nats_client: NATSClient = None
        self.redis_client: redis.Redis = None
        self.nats_url = os.getenv('NATS_URL', 'nats://localhost:4222')
        self.redis_host = os.getenv('REDIS_HOST', 'localhost')
        self.redis_port = int(os.getenv('REDIS_PORT', '6379'))
        self.redis_password = os.getenv('REDIS_PASSWORD', '')

    async def connect(self):
        """Connect to NATS and Redis"""
        # Connect to NATS
        logger.info(f"Connecting to NATS at {self.nats_url}...")
        self.nats_client = await nats.connect(
            servers=[self.nats_url],
            max_reconnect_attempts=-1,
            reconnect_time_wait=2,
            disconnected_cb=self._on_nats_disconnected,
            reconnected_cb=self._on_nats_reconnected,
            error_cb=self._on_nats_error
        )
        logger.info("Connected to NATS")

        # Connect to Redis
        logger.info(f"Connecting to Redis at {self.redis_host}:{self.redis_port}...")
        self.redis_client = redis.Redis(
            host=self.redis_host,
            port=self.redis_port,
            password=self.redis_password if self.redis_password else None,
            decode_responses=True
        )
        await self.redis_client.ping()
        logger.info("Connected to Redis")

    async def _on_nats_disconnected(self):
        logger.warning("Disconnected from NATS")

    async def _on_nats_reconnected(self):
        logger.info("Reconnected to NATS")

    async def _on_nats_error(self, e):
        logger.error(f"NATS error: {e}")

    async def get_active_pods(self, session_id: str) -> List[str]:
        """Query Redis to get active pods for a session"""
        try:
            key = f"session:connections:{session_id}"
            members = await self.redis_client.smembers(key)

            # Extract unique pod IDs from members (format: pod_id:connection_id)
            pod_ids = set()
            for member in members:
                # Split and take everything before the last part (connection ID)
                parts = member.split(':')
                if len(parts) >= 2:
                    pod_id = ':'.join(parts[:-1])  # Everything except last part
                    pod_ids.add(pod_id)

            return list(pod_ids)
        except Exception as e:
            logger.error(f"Failed to get active pods for session {session_id}: {e}")
            return []

    async def simulate_ai_response(self, message: str) -> str:
        """Simulate AI processing (in real implementation, this would call LangGraph/LLM)"""
        # For demo, generate a simple response
        responses = [
            f"I received your message: '{message}'",
            "This is a demo response from the LangGraph workflow service.",
            "In a real implementation, this would integrate with AI models like GPT-4, Claude, etc.",
            "The response is being streamed token by token to demonstrate the chunk buffering system.",
        ]
        return " ".join(responses)

    async def process_workflow_request(self, msg):
        """Process a workflow execution request"""
        try:
            # Parse the workflow request
            data = json.loads(msg.data.decode())

            message_id = data.get('message_id')
            session_id = data.get('session_id')
            user_id = data.get('user_id')
            user_message = data.get('message')
            correlation_id = data.get('correlation_id')

            logger.info(f"Processing workflow request: session={session_id}, message_id={message_id}")

            # Get active pods for this session
            active_pods = await self.get_active_pods(session_id)

            if not active_pods:
                logger.warning(f"No active pods found for session {session_id}")
                # Could use broadcast subject as fallback
                active_pods = []
                broadcast_subject = f"chat.session.{session_id}.broadcast"
            else:
                logger.info(f"Found {len(active_pods)} active pods for session {session_id}: {active_pods}")
                broadcast_subject = None

            # Simulate AI response generation
            full_response = await self.simulate_ai_response(user_message)

            # Split response into chunks (simulate streaming tokens)
            words = full_response.split()
            chunk_size = 3  # words per chunk
            chunks = [' '.join(words[i:i + chunk_size]) for i in range(0, len(words), chunk_size)]

            # Stream chunks
            for chunk_id, chunk_text in enumerate(chunks):
                is_final = (chunk_id == len(chunks) - 1)

                # Create response chunk
                response_chunk = {
                    "session_id": session_id,
                    "message_id": message_id,
                    "chunk_id": chunk_id,
                    "chunk": chunk_text + (" " if not is_final else ""),
                    "chunk_type": "content",
                    "is_final": is_final,
                    "timestamp": datetime.utcnow().isoformat() + "Z",
                    "correlation_id": correlation_id
                }

                # Add metadata to final chunk
                if is_final:
                    response_chunk["metadata"] = {
                        "model": "demo-gpt",
                        "tokens_used": len(words),
                        "finish_reason": "stop"
                    }

                chunk_json = json.dumps(response_chunk).encode()

                # Publish to active pods
                for pod_id in active_pods:
                    subject = f"chat.pod.{pod_id}.response"
                    await self.nats_client.publish(subject, chunk_json)
                    logger.debug(f"Published chunk {chunk_id} to {subject}")

                # Also publish to broadcast if no active pods
                if broadcast_subject and not active_pods:
                    await self.nats_client.publish(broadcast_subject, chunk_json)
                    logger.debug(f"Published chunk {chunk_id} to broadcast")

                # Simulate streaming delay (realistic token generation)
                await asyncio.sleep(0.1)

            logger.info(f"Completed workflow for message_id={message_id}, sent {len(chunks)} chunks")

        except Exception as e:
            logger.error(f"Error processing workflow request: {e}", exc_info=True)

    async def subscribe_to_workflow_requests(self):
        """Subscribe to workflow execution requests"""
        subject = "chat.workflow.execute.*"
        logger.info(f"Subscribing to {subject}")

        await self.nats_client.subscribe(
            subject,
            cb=self.process_workflow_request
        )

        logger.info(f"Subscribed to {subject} - ready to process workflows")

    async def run(self):
        """Main run loop"""
        await self.connect()
        await self.subscribe_to_workflow_requests()

        # Keep running
        logger.info("Workflow service is running. Press Ctrl+C to stop.")
        try:
            while True:
                await asyncio.sleep(1)
        except KeyboardInterrupt:
            logger.info("Shutting down...")
        finally:
            await self.shutdown()

    async def shutdown(self):
        """Clean shutdown"""
        if self.nats_client:
            await self.nats_client.drain()
            await self.nats_client.close()
        if self.redis_client:
            await self.redis_client.close()
        logger.info("Shutdown complete")


async def main():
    """Entry point"""
    logger.info("Starting Demo LangGraph Workflow Service")
    service = WorkflowService()
    await service.run()


if __name__ == "__main__":
    asyncio.run(main())
