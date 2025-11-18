/**
 * API Client for Chat Backend
 */

const API_BASE_URL = process.env.REACT_APP_API_URL || 'http://localhost';

/**
 * Submit a chat message
 */
export async function sendMessage(sessionId, userId, message) {
  const response = await fetch(`${API_BASE_URL}/api/chat`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      session_id: sessionId,
      user_id: userId,
      message: message,
    }),
  });

  if (!response.ok) {
    throw new Error(`Failed to send message: ${response.status}`);
  }

  return response.json();
}

/**
 * Get SSE connection URL
 */
export function getSSEUrl(sessionId, userId) {
  return `${API_BASE_URL}/api/sse?session_id=${sessionId}&user_id=${userId}`;
}

/**
 * Check backend health
 */
export async function checkHealth() {
  const response = await fetch(`${API_BASE_URL}/health`);

  if (!response.ok) {
    throw new Error(`Health check failed: ${response.status}`);
  }

  return response.json();
}
