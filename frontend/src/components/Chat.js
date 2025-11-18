import React, { useState, useEffect, useRef } from 'react';
import { sendMessage } from '../services/api';
import { useSSE } from '../hooks/useSSE';
import './Chat.css';

function Chat({ sessionId, userId }) {
  const [messages, setMessages] = useState([]);
  const [inputMessage, setInputMessage] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [currentResponse, setCurrentResponse] = useState('');
  const [currentMessageId, setCurrentMessageId] = useState(null);
  const messagesEndRef = useRef(null);

  const { connectionStatus, lastChunk } = useSSE(sessionId, userId);

  // Handle incoming SSE chunks
  useEffect(() => {
    if (!lastChunk) return;

    if (lastChunk.type === 'complete') {
      // Message completed
      if (currentResponse) {
        setMessages((prev) => [
          ...prev,
          {
            id: lastChunk.message_id,
            role: 'assistant',
            content: currentResponse,
            timestamp: new Date(),
            tokenCount: lastChunk.token_count,
          },
        ]);
        setCurrentResponse('');
        setCurrentMessageId(null);
      }
    } else if (lastChunk.chunk_id !== undefined) {
      // New chunk received
      if (lastChunk.message_id !== currentMessageId) {
        // New message started
        setCurrentMessageId(lastChunk.message_id);
        setCurrentResponse(lastChunk.chunk);
      } else {
        // Continue existing message
        setCurrentResponse((prev) => prev + lastChunk.chunk);
      }
    }
  }, [lastChunk, currentMessageId, currentResponse]);

  // Auto-scroll to bottom
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, currentResponse]);

  const handleSubmit = async (e) => {
    e.preventDefault();

    if (!inputMessage.trim() || isSubmitting) {
      return;
    }

    const userMessage = inputMessage.trim();
    setInputMessage('');
    setIsSubmitting(true);

    // Add user message to chat
    const newUserMessage = {
      id: Date.now(),
      role: 'user',
      content: userMessage,
      timestamp: new Date(),
    };
    setMessages((prev) => [...prev, newUserMessage]);

    try {
      // Send message to backend
      const response = await sendMessage(sessionId, userId, userMessage);
      console.log('Message sent:', response);
    } catch (error) {
      console.error('Failed to send message:', error);
      // Show error in chat
      setMessages((prev) => [
        ...prev,
        {
          id: Date.now(),
          role: 'error',
          content: `Error: ${error.message}`,
          timestamp: new Date(),
        },
      ]);
    } finally {
      setIsSubmitting(false);
    }
  };

  const getStatusColor = () => {
    switch (connectionStatus) {
      case 'connected':
        return '#10b981';
      case 'connecting':
        return '#f59e0b';
      case 'error':
        return '#ef4444';
      default:
        return '#6b7280';
    }
  };

  return (
    <div className="chat-container">
      <div className="chat-header">
        <div className="chat-title">
          <h2>Universal Chat Interface</h2>
          <div className="session-info">
            <span>Session: {sessionId}</span>
            <span className="connection-status">
              <span
                className="status-dot"
                style={{ backgroundColor: getStatusColor() }}
              />
              {connectionStatus}
            </span>
          </div>
        </div>
      </div>

      <div className="chat-messages">
        {messages.map((msg) => (
          <div key={msg.id} className={`message ${msg.role}`}>
            <div className="message-header">
              <span className="message-role">
                {msg.role === 'user' ? 'You' : msg.role === 'assistant' ? 'AI' : 'System'}
              </span>
              <span className="message-time">
                {msg.timestamp.toLocaleTimeString()}
              </span>
            </div>
            <div className="message-content">{msg.content}</div>
            {msg.tokenCount && (
              <div className="message-footer">
                <span className="token-count">{msg.tokenCount} tokens</span>
              </div>
            )}
          </div>
        ))}

        {currentResponse && (
          <div className="message assistant streaming">
            <div className="message-header">
              <span className="message-role">AI</span>
              <span className="message-time">
                <span className="streaming-indicator">●</span> Streaming...
              </span>
            </div>
            <div className="message-content">{currentResponse}</div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      <form className="chat-input-form" onSubmit={handleSubmit}>
        <input
          type="text"
          value={inputMessage}
          onChange={(e) => setInputMessage(e.target.value)}
          placeholder="Type your message..."
          disabled={isSubmitting || connectionStatus !== 'connected'}
          className="chat-input"
        />
        <button
          type="submit"
          disabled={isSubmitting || !inputMessage.trim() || connectionStatus !== 'connected'}
          className="send-button"
        >
          {isSubmitting ? 'Sending...' : 'Send'}
        </button>
      </form>
    </div>
  );
}

export default Chat;
