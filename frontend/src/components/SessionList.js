import React from 'react';
import './SessionList.css';

function SessionList({ sessions, currentSessionId, onSelectSession, onNewSession }) {
  return (
    <div className="session-list">
      <div className="session-list-header">
        <h3>Chat Sessions</h3>
        <button onClick={onNewSession} className="new-session-button">
          + New Chat
        </button>
      </div>

      <div className="sessions">
        {sessions.length === 0 ? (
          <div className="no-sessions">
            <p>No chat sessions yet</p>
            <p className="hint">Click "New Chat" to start</p>
          </div>
        ) : (
          sessions.map((session) => (
            <div
              key={session.id}
              className={`session-item ${session.id === currentSessionId ? 'active' : ''}`}
              onClick={() => onSelectSession(session.id)}
            >
              <div className="session-item-header">
                <span className="session-title">
                  {session.title || `Session ${session.id.substring(0, 8)}`}
                </span>
                <span className="session-time">
                  {new Date(session.createdAt).toLocaleDateString()}
                </span>
              </div>
              {session.lastMessage && (
                <div className="session-preview">
                  {session.lastMessage.substring(0, 50)}
                  {session.lastMessage.length > 50 ? '...' : ''}
                </div>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
}

export default SessionList;
