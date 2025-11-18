import { useEffect, useRef, useState } from 'react';
import { getSSEUrl } from '../services/api';

/**
 * Hook to manage SSE connection for streaming responses
 */
export function useSSE(sessionId, userId) {
  const [connectionStatus, setConnectionStatus] = useState('disconnected');
  const [lastChunk, setLastChunk] = useState(null);
  const eventSourceRef = useRef(null);

  useEffect(() => {
    if (!sessionId || !userId) {
      return;
    }

    // Create SSE connection
    const url = getSSEUrl(sessionId, userId);
    const eventSource = new EventSource(url);
    eventSourceRef.current = eventSource;

    setConnectionStatus('connecting');

    // Connection opened
    eventSource.onopen = () => {
      console.log('SSE connection opened');
      setConnectionStatus('connected');
    };

    // Handle 'connected' event
    eventSource.addEventListener('connected', (event) => {
      const data = JSON.parse(event.data);
      console.log('Connected to SSE:', data);
    });

    // Handle 'chunk' events
    eventSource.addEventListener('chunk', (event) => {
      const chunk = JSON.parse(event.data);
      console.log('Received chunk:', chunk);
      setLastChunk(chunk);
    });

    // Handle 'message_complete' events
    eventSource.addEventListener('message_complete', (event) => {
      const data = JSON.parse(event.data);
      console.log('Message complete:', data);
      setLastChunk({ ...data, type: 'complete' });
    });

    // Handle 'ping' events (heartbeat)
    eventSource.addEventListener('ping', (event) => {
      // Just acknowledge heartbeat
      console.debug('Heartbeat received');
    });

    // Error handling
    eventSource.onerror = (error) => {
      console.error('SSE error:', error);
      setConnectionStatus('error');
    };

    // Cleanup on unmount
    return () => {
      console.log('Closing SSE connection');
      eventSource.close();
      setConnectionStatus('disconnected');
    };
  }, [sessionId, userId]);

  return {
    connectionStatus,
    lastChunk,
  };
}
