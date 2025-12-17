import { useState, useEffect, useRef, useCallback } from 'react';
import { deriveDisplayStatus, isActiveStatus } from './statusUtils';
import { API } from '../../helpers/api';

const WS_MESSAGE_TYPES = {
  NEW: 'new',
  UPDATE: 'update',
  DELETE: 'delete',
  SNAPSHOT: 'snapshot',
  CHANNEL: 'channel_update',
};

const useMonitorWs = () => {
  const [summaries, setSummaries] = useState([]);
  const [stats, setStats] = useState({ total: 0, active: 0, memory: 0 });
  const [connected, setConnected] = useState(false);
  const [channelUpdates, setChannelUpdates] = useState({});
  const wsRef = useRef(null);
  const reconnectTimeoutRef = useRef(null);
  const reconnectAttempts = useRef(0);
  const disconnectTimerRef = useRef(null);
  const stableOpenTimerRef = useRef(null);
  const statsIntervalRef = useRef(null);
  const maxReconnectAttempts = 10;
  const baseReconnectDelay = 1000;

  const calculateStats = useCallback((reqs) => {
    const active = reqs.filter((r) => isActiveStatus(deriveDisplayStatus(r))).length;
    return { total: reqs.length, active };
  }, []);

  const fetchStats = useCallback(async () => {
    try {
      const response = await API.get('/api/monitor/stats');
      const { success, data } = response.data;
      if (success && data) {
        setStats({
          total: data.total_requests || 0,
          active: data.active_requests || 0,
          memory: data.memory_bytes || 0,
        });
      }
    } catch (error) {
      // Silently fail - stats will be updated on next interval
    }
  }, []);

  const buildWsUrl = useCallback(() => {
    const envBase = import.meta.env.VITE_REACT_APP_SERVER_URL;

    if (envBase) {
      try {
        const parsed = new URL(envBase);
        const protocol = parsed.protocol === 'https:' ? 'wss:' : 'ws:';
        const basePath = parsed.pathname.replace(/\/$/, '');
        return `${protocol}//${parsed.host}${basePath}/api/monitor/ws`;
      } catch (error) {
        console.warn('Invalid VITE_REACT_APP_SERVER_URL for monitor websocket:', error);
      }
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/api/monitor/ws`;
  }, []);

  const handleMessage = useCallback((event) => {
    try {
      // Handle multiple messages separated by newlines
      const messages = event.data.split('\n').filter((m) => m.trim());

      messages.forEach((msgStr) => {
        const message = JSON.parse(msgStr);

        switch (message.type) {
          case WS_MESSAGE_TYPES.SNAPSHOT:
            // Initial snapshot of all summaries
            const snapshotData = Array.isArray(message.payload)
              ? message.payload
              : [];
            setSummaries(snapshotData);
            setStats((prev) => ({ ...prev, ...calculateStats(snapshotData) }));
            break;

          case WS_MESSAGE_TYPES.NEW:
            // New request summary added
            setSummaries((prev) => {
              const newSummaries = [...prev, message.payload];
              // Keep only last 100 records on frontend too
              if (newSummaries.length > 100) {
                newSummaries.shift();
              }
              setStats((prevStats) => ({ ...prevStats, ...calculateStats(newSummaries) }));
              return newSummaries;
            });
            break;

          case WS_MESSAGE_TYPES.UPDATE:
            // Existing request summary updated
            setSummaries((prev) => {
              const updated = prev.map((r) =>
                r.id === message.payload.id ? message.payload : r
              );
              setStats((prevStats) => ({ ...prevStats, ...calculateStats(updated) }));
              return updated;
            });
            break;

          case WS_MESSAGE_TYPES.DELETE:
            // Request deleted (shouldn't happen often)
            setSummaries((prev) => {
              const filtered = prev.filter((r) => r.id !== message.payload.id);
              setStats((prevStats) => ({ ...prevStats, ...calculateStats(filtered) }));
              return filtered;
            });
            break;

          case WS_MESSAGE_TYPES.CHANNEL:
            if (message.payload && message.payload.request_id) {
              setChannelUpdates((prev) => ({
                ...prev,
                [message.payload.request_id]: message.payload,
              }));
            }
            break;

          default:
            console.warn('Unknown message type:', message.type);
        }
      });
    } catch (error) {
      console.error('Error parsing WebSocket message:', error);
    }
  }, [calculateStats]);

  const connect = useCallback(() => {
    // Clear any existing reconnect timeout
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    // Close existing connection if any
    if (wsRef.current) {
      wsRef.current.close();
    }

    // Determine WebSocket URL
    const wsUrl = buildWsUrl();

    try {
      const ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        if (disconnectTimerRef.current) {
          clearTimeout(disconnectTimerRef.current);
          disconnectTimerRef.current = null;
        }
        setConnected(true);

        // Only reset attempts after the connection stays up for a bit to avoid flapping
        if (stableOpenTimerRef.current) {
          clearTimeout(stableOpenTimerRef.current);
        }
        stableOpenTimerRef.current = setTimeout(() => {
          reconnectAttempts.current = 0;
        }, 3000);
      };

      ws.onmessage = handleMessage;

      ws.onclose = (event) => {
        if (stableOpenTimerRef.current) {
          clearTimeout(stableOpenTimerRef.current);
          stableOpenTimerRef.current = null;
        }

        if (disconnectTimerRef.current) {
          clearTimeout(disconnectTimerRef.current);
        }
        disconnectTimerRef.current = setTimeout(() => setConnected(false), 800);

        // Attempt to reconnect with exponential backoff
        if (reconnectAttempts.current < maxReconnectAttempts) {
          const delay = Math.min(
            baseReconnectDelay * Math.pow(2, reconnectAttempts.current),
            30000
          );
          reconnectTimeoutRef.current = setTimeout(() => {
            reconnectAttempts.current++;
            connect();
          }, delay);
        }
      };

      ws.onerror = (error) => {
        // WebSocket error occurred
      };

      wsRef.current = ws;
    } catch (error) {
      // Failed to create WebSocket
    }
  }, [handleMessage, buildWsUrl]);

  const reconnect = useCallback(() => {
    reconnectAttempts.current = 0;
    connect();
  }, [connect]);

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (disconnectTimerRef.current) {
        clearTimeout(disconnectTimerRef.current);
      }
      if (stableOpenTimerRef.current) {
        clearTimeout(stableOpenTimerRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [connect]);

  useEffect(() => {
    // Fetch stats immediately
    fetchStats();

    // Set up interval to fetch stats every 2 seconds
    statsIntervalRef.current = setInterval(fetchStats, 2000);

    return () => {
      if (statsIntervalRef.current) {
        clearInterval(statsIntervalRef.current);
      }
    };
  }, [fetchStats]);

  return {
    summaries,
    stats,
    connected,
    reconnect,
    channelUpdates,
  };
};

export default useMonitorWs;
