# Universal Chat Interface - Frontend

React-based frontend for the Universal Chat Interface with real-time SSE streaming and session management.

## Features

- **Real-time Streaming**: SSE-based streaming for live AI responses
- **Session Management**: Create and manage multiple chat sessions
- **Session History**: View and switch between previous chat sessions
- **Message Persistence**: Sessions stored in browser localStorage
- **Connection Status**: Visual indicators for SSE connection state
- **Responsive UI**: Clean, modern interface with gradient theme
- **Auto-generated User IDs**: Persistent user identification across sessions

## Architecture

### Components

```
src/
├── components/
│   ├── Chat.js              # Main chat interface
│   ├── Chat.css             # Chat component styles
│   ├── SessionList.js       # Session management sidebar
│   └── SessionList.css      # SessionList styles
├── hooks/
│   └── useSSE.js            # Custom hook for SSE connections
├── services/
│   └── api.js               # Backend API client
├── App.js                   # Root component with session state
├── App.css                  # App-level styles
├── index.js                 # React entry point
└── index.css                # Global styles
```

### Key Components

#### Chat Component
- Displays message history for current session
- Shows streaming AI responses in real-time
- Handles message submission via POST /api/chat
- Auto-scrolls to latest messages
- Visual streaming indicator during AI response

#### SessionList Component
- Lists all chat sessions with timestamps
- Shows last message preview for each session
- "New Chat" button to create sessions
- Session selection with active state highlighting

#### useSSE Hook
- Manages EventSource SSE connection lifecycle
- Handles connection events (connected, chunk, error)
- Auto-reconnects on disconnect
- Provides connection status and last chunk data

### State Management

**User ID**: Generated once and stored in localStorage
```javascript
user-${timestamp}-${randomString}
```

**Sessions**: Array stored in localStorage
```javascript
{
  id: string,           // Session identifier
  name: string,         // Display name
  messages: [],         // Message history
  lastMessage: string,  // Last message preview
  timestamp: number     // Creation time
}
```

## API Integration

### Endpoints Used

**POST /api/chat**
- Submit new chat message
- Returns: `{ message_id, correlation_id }`

**GET /api/sse**
- Establish SSE connection
- Query params: `session_id`, `user_id`
- Events: `connected`, `chunk`, `message_complete`, `error`

**GET /health**
- Health check endpoint

### SSE Event Handling

```javascript
// Connected event
{
  "connection_id": "abc-123",
  "session_id": "session-id"
}

// Chunk event
{
  "session_id": "session-id",
  "message_id": "msg-123",
  "chunk_id": 0,
  "chunk": "Response text",
  "is_final": false
}

// Message complete event
{
  "session_id": "session-id",
  "message_id": "msg-123",
  "is_final": true,
  "metadata": {
    "tokens_used": 42,
    "model": "demo"
  }
}
```

## Development

### Prerequisites

- Node.js 18+ and npm
- Backend services running (see root README.md)

### Installation

```bash
cd frontend
npm install
```

### Environment Variables

Create `.env` file:

```bash
# Backend API URL
REACT_APP_API_URL=http://localhost
```

### Development Server

```bash
npm start
```

Runs on http://localhost:3000

### Build for Production

```bash
npm run build
```

Creates optimized production build in `build/`

## Docker Deployment

### Build Docker Image

```bash
docker build -t chat-frontend .
```

### Run Container

```bash
docker run -p 3000:80 \
  -e REACT_APP_API_URL=http://localhost \
  chat-frontend
```

### Using Docker Compose

Frontend is included in the main docker-compose.yml:

```bash
# From project root
docker-compose up -d frontend
```

Access at http://localhost:3000

## Usage

### Creating a Session

1. Click "New Chat" in the sidebar
2. A new session is automatically created
3. Session is added to localStorage

### Sending Messages

1. Type message in the input field
2. Click "Send" or press Enter
3. Message appears in chat history
4. SSE connection receives streaming response

### Viewing Session History

1. Previous sessions appear in left sidebar
2. Click any session to switch to it
3. Session messages are loaded from localStorage
4. Last message preview shown for each session

### Connection States

- **Connected**: Green indicator, SSE active
- **Connecting**: Yellow indicator, establishing connection
- **Disconnected**: Red indicator, connection failed

## Troubleshooting

### SSE Connection Fails

**Problem**: "SSE Error" or connection status stuck on "connecting"

**Solutions**:
1. Verify backend is running: `curl http://localhost/health`
2. Check browser console for CORS errors
3. Ensure REACT_APP_API_URL is set correctly
4. Check nginx configuration allows SSE

### Messages Not Streaming

**Problem**: Messages sent but no response chunks received

**Solutions**:
1. Check workflow service is running: `docker-compose logs workflow`
2. Verify NATS connectivity: `docker-compose logs backend-1`
3. Check browser console for SSE events
4. Ensure session_id matches between POST and SSE connection

### Sessions Not Persisting

**Problem**: Sessions disappear after browser refresh

**Solutions**:
1. Check browser localStorage is enabled
2. Verify no localStorage quota exceeded
3. Check browser console for errors during localStorage.setItem

### CORS Errors

**Problem**: Cross-origin request blocked

**Solutions**:
1. Ensure backend CORS headers are set correctly
2. Check nginx.conf allows proper origins
3. For development, ensure REACT_APP_API_URL matches backend URL

## Architecture Decisions

### Why localStorage?

- No backend session management needed
- Works offline for viewing history
- Simple implementation
- User-specific data only

### Why Custom SSE Hook?

- Reusable across components
- Handles connection lifecycle
- Automatic cleanup
- Error handling built-in

### Why EventSource?

- Native browser API for SSE
- Auto-reconnection
- Simple event-based interface
- Better than long polling

### Why Not WebSocket?

- SSE is simpler for unidirectional streaming
- Built-in reconnection
- Works through firewalls/proxies
- HTTP/2 multiplexing support

## Performance Considerations

### Message Rendering

- Uses React key props for efficient updates
- Auto-scroll only when near bottom
- Chunk concatenation in state (not DOM)

### Connection Management

- Single SSE connection per session
- Cleanup on component unmount
- EventSource auto-reconnect

### localStorage

- Sessions stored as JSON
- Updates batched (not per-chunk)
- No limit on message history (browser-dependent)

## Browser Support

- Chrome 90+
- Firefox 88+
- Safari 14+
- Edge 90+

Requires:
- EventSource API
- localStorage API
- CSS Grid support

## Future Enhancements

- [ ] Message search functionality
- [ ] Export chat history
- [ ] Session deletion
- [ ] User profile management
- [ ] Dark mode toggle
- [ ] Message editing/regeneration
- [ ] File upload support
- [ ] Markdown rendering for AI responses
- [ ] Code syntax highlighting
- [ ] Mobile-responsive design improvements
- [ ] Offline mode with service worker
- [ ] Session syncing across devices (backend integration)

## Testing

### Manual Testing

1. Start all services via docker-compose
2. Open http://localhost:3000
3. Create new session
4. Send test message
5. Verify streaming response appears
6. Refresh page and verify session persists
7. Create multiple sessions and switch between them

### E2E Tests

See `/tests/README.md` for comprehensive E2E tests that include frontend validation.

## License

MIT
