import React, { useState, useEffect } from 'react';
import Chat from './components/Chat';
import SessionList from './components/SessionList';
import './App.css';

function App() {
  const [userId] = useState(() => {
    // Get or create user ID
    let id = localStorage.getItem('userId');
    if (!id) {
      id = `user-${Date.now()}-${Math.random().toString(36).substring(7)}`;
      localStorage.setItem('userId', id);
    }
    return id;
  });

  const [sessions, setSessions] = useState(() => {
    // Load sessions from localStorage
    const saved = localStorage.getItem('chatSessions');
    return saved ? JSON.parse(saved) : [];
  });

  const [currentSessionId, setCurrentSessionId] = useState(() => {
    // Load current session from localStorage or use first session
    const savedSessionId = localStorage.getItem('currentSessionId');
    if (savedSessionId && sessions.find((s) => s.id === savedSessionId)) {
      return savedSessionId;
    }
    return sessions.length > 0 ? sessions[0].id : null;
  });

  // Save sessions to localStorage whenever they change
  useEffect(() => {
    localStorage.setItem('chatSessions', JSON.stringify(sessions));
  }, [sessions]);

  // Save current session ID to localStorage
  useEffect(() => {
    if (currentSessionId) {
      localStorage.setItem('currentSessionId', currentSessionId);
    }
  }, [currentSessionId]);

  const handleNewSession = () => {
    const newSession = {
      id: `session-${Date.now()}-${Math.random().toString(36).substring(7)}`,
      title: `Chat ${sessions.length + 1}`,
      createdAt: new Date().toISOString(),
      lastMessage: null,
    };

    setSessions((prev) => [newSession, ...prev]);
    setCurrentSessionId(newSession.id);
  };

  const handleSelectSession = (sessionId) => {
    setCurrentSessionId(sessionId);
  };

  // Create initial session if none exist
  useEffect(() => {
    if (sessions.length === 0) {
      handleNewSession();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="app">
      <SessionList
        sessions={sessions}
        currentSessionId={currentSessionId}
        onSelectSession={handleSelectSession}
        onNewSession={handleNewSession}
      />
      <div className="chat-area">
        {currentSessionId ? (
          <Chat sessionId={currentSessionId} userId={userId} />
        ) : (
          <div className="no-session">
            <h2>No session selected</h2>
            <p>Create a new chat session to get started</p>
          </div>
        )}
      </div>
    </div>
  );
}

export default App;
