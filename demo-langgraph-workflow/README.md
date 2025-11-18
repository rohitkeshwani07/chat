# Demo LangGraph Workflow Service

A demo workflow service that simulates AI response generation and demonstrates the complete message flow in the distributed chat system.

## Overview

This service completes the chat architecture loop by:

1. **Subscribing** to workflow execution requests from backend pods (via NATS)
2. **Simulating** AI response generation (in production, this would integrate with LangGraph/LLM)
3. **Streaming** response chunks back to backend pods
4. **Querying** Redis to find which pods have active SSE connections
5. **Publishing** chunks to the appropriate pod-specific NATS subjects

## Architecture

```
┌──────────────┐
│ Backend Pods │
└──────┬───────┘
       │ Publish: chat.workflow.execute.{sessionId}
       │
       ▼
┌─────────────┐
│    NATS     │
│   Broker    │
└──────┬──────┘
       │ Subscribe
       │
       ▼
┌──────────────────┐       ┌─────────┐
│ Workflow Service │◄──────│  Redis  │
│  (This Service)  │       │ Session │
└──────┬───────────┘       │Registry │
       │                   └─────────┘
       │ Publish: chat.pod.{podId}.response
       │
       ▼
┌─────────────┐
│    NATS     │
│   Broker    │
└──────┬──────┘
       │ Subscribe
       │
       ▼
┌──────────────┐
│ Backend Pods │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ SSE Client   │
└──────────────┘
```

## How It Works

### 1. Workflow Request Reception

Listens on NATS subject: `chat.workflow.execute.*`

Receives messages like:
```json
{
  "message_id": "msg_abc123",
  "session_id": "sess_456",
  "user_id": "user_789",
  "message": "What is the weather today?",
  "context": {
    "ai_provider": "openai",
    "model": "gpt-4"
  },
  "timestamp": "2025-11-18T20:30:00Z",
  "correlation_id": "corr_xyz"
}
```

### 2. Session Registry Lookup

Queries Redis to find active pods for the session:
```python
key = "session:connections:{session_id}"
members = redis.smembers(key)
# Returns: ["pod-1:conn-abc", "pod-2:conn-xyz"]
```

Extracts unique pod IDs: `["pod-1", "pod-2"]`

### 3. AI Response Simulation

For demo purposes, generates a canned response. In production, this would:
- Integrate with LangGraph for workflow orchestration
- Call LLM APIs (OpenAI, Anthropic, etc.)
- Stream tokens as they're generated
- Handle tool calls, function execution, etc.

### 4. Chunk Streaming

Splits response into chunks and publishes to NATS:

```python
# For each chunk
response_chunk = {
    "session_id": "sess_456",
    "message_id": "msg_abc123",
    "chunk_id": 0,
    "chunk": "Hello",
    "chunk_type": "content",
    "is_final": False,
    "timestamp": "2025-11-18T20:30:05Z",
    "correlation_id": "corr_xyz"
}

# Publish to each active pod
for pod_id in ["pod-1", "pod-2"]:
    subject = f"chat.pod.{pod_id}.response"
    nats.publish(subject, json.dumps(response_chunk))
```

### 5. Final Chunk with Metadata

The last chunk includes metadata:
```json
{
  "chunk_id": 5,
  "chunk": "!",
  "is_final": true,
  "metadata": {
    "model": "demo-gpt",
    "tokens_used": 42,
    "finish_reason": "stop"
  }
}
```

## Demo Response

Current demo response:
```
"I received your message: '{user_message}'
This is a demo response from the LangGraph workflow service.
In a real implementation, this would integrate with AI models like GPT-4, Claude, etc.
The response is being streamed token by token to demonstrate the chunk buffering system."
```

Streamed in chunks of 3 words with 100ms delay between chunks.

## Configuration

Environment variables:

```bash
NATS_URL=nats://localhost:4222         # NATS server URL
REDIS_HOST=localhost                    # Redis host
REDIS_PORT=6379                         # Redis port
REDIS_PASSWORD=                         # Redis password (if any)
```

## Running Locally

### Prerequisites

- Python 3.11+
- NATS server running
- Redis running

### Install Dependencies

```bash
cd demo-langgraph-workflow
pip install -r requirements.txt
```

### Run

```bash
python src/workflow_service.py
```

### With Docker

```bash
# Build
docker build -t chat-workflow .

# Run
docker run \
  -e NATS_URL=nats://nats:4222 \
  -e REDIS_HOST=redis \
  chat-workflow
```

### With Docker Compose

```bash
# From project root
docker-compose up -d workflow

# View logs
docker-compose logs -f workflow
```

## Testing End-to-End

1. Start all services:
```bash
docker-compose up -d
```

2. Open SSE connection (Terminal 1):
```bash
curl -N "http://localhost/api/sse?session_id=test&user_id=user1"
```

3. Send chat message (Terminal 2):
```bash
curl -X POST http://localhost/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "test",
    "user_id": "user1",
    "message": "Hello, how are you?"
  }'
```

4. Watch Terminal 1 receive streaming chunks!

Expected output in Terminal 1:
```
event: connected
data: {"connection_id":"...","session_id":"test"}

event: chunk
data: {"session_id":"test","message_id":"...","chunk_id":0,"chunk":"I received your ","is_final":false}

event: chunk
data: {"session_id":"test","message_id":"...","chunk_id":1,"chunk":"message: 'Hello, how ","is_final":false}

...

event: message_complete
data: {"message_id":"...","token_count":42}
```

## Extending for Production

To integrate with real AI models:

### 1. LangGraph Integration

```python
from langgraph.graph import StateGraph

# Define workflow graph
workflow = StateGraph(state_schema)

# Add nodes
workflow.add_node("analyze_intent", analyze_intent_node)
workflow.add_node("call_llm", call_llm_node)
workflow.add_node("format_response", format_response_node)

# Define edges
workflow.add_edge("analyze_intent", "call_llm")
workflow.add_edge("call_llm", "format_response")

# Compile
app = workflow.compile()

# Execute
async for chunk in app.astream(input_data):
    # Publish chunk to NATS
    await publish_chunk(chunk)
```

### 2. LLM Integration (OpenAI)

```python
from openai import AsyncOpenAI

client = AsyncOpenAI(api_key=os.getenv("OPENAI_API_KEY"))

async for chunk in await client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": user_message}],
    stream=True
):
    if chunk.choices[0].delta.content:
        await publish_chunk(chunk.choices[0].delta.content)
```

### 3. Anthropic Claude

```python
from anthropic import AsyncAnthropic

client = AsyncAnthropic(api_key=os.getenv("ANTHROPIC_API_KEY"))

async with client.messages.stream(
    model="claude-3-sonnet-20240229",
    messages=[{"role": "user", "content": user_message}],
    max_tokens=1024
) as stream:
    async for text in stream.text_stream:
        await publish_chunk(text)
```

## Monitoring

Check service logs:
```bash
docker-compose logs -f workflow
```

Verify NATS subscriptions:
```bash
# Access NATS monitoring
curl http://localhost:8222/connz
curl http://localhost:8222/subsz
```

## Next Steps

- [ ] Integrate actual LangGraph workflows
- [ ] Add LLM provider support (OpenAI, Anthropic, etc.)
- [ ] Implement conversation history management
- [ ] Add tool/function calling support
- [ ] Implement error handling and retries
- [ ] Add metrics and monitoring
- [ ] Support multiple concurrent workflows
- [ ] Add workflow state persistence
