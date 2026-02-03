/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useState, useEffect, useRef, useCallback } from 'react';
import { API } from '../../helpers/api';
import {
  MAX_RECONNECT_ATTEMPTS,
  BASE_RECONNECT_DELAY_MS,
  STABLE_CONNECTION_TIMEOUT_MS,
  MAX_SUMMARY_ITEMS,
} from './constants';

const WS_MESSAGE_TYPES = {
  NEW: 'new',
  UPDATE: 'update',
  DELETE: 'delete',
  SNAPSHOT: 'snapshot',
  CHANNEL: 'channel_update',
};

const useMonitorWs = ({ focusedRequestId } = {}) => {
  const [summaries, setSummaries] = useState([]);
  const [stats, setStats] = useState({ total: 0, active: 0, memory: 0 });
  const [connected, setConnected] = useState(false);
  // Only keep the most recent channel update for the currently focused request
  // to avoid unbounded memory growth when the page is open for long periods.
  const [channelUpdate, setChannelUpdate] = useState(null);
  const wsRef = useRef(null);
  const reconnectTimeoutRef = useRef(null);
  const reconnectAttempts = useRef(0);
  const disconnectTimerRef = useRef(null);
  const stableOpenTimerRef = useRef(null);
  const statsIntervalRef = useRef(null);
  const focusedRequestIdRef = useRef(focusedRequestId ?? null);
  const maxReconnectAttempts = MAX_RECONNECT_ATTEMPTS;
  const baseReconnectDelay = BASE_RECONNECT_DELAY_MS;

  const trimSummaries = useCallback((list) => {
    if (!Array.isArray(list)) return [];
    if (list.length <= MAX_SUMMARY_ITEMS) return list;
    const sortedByTime = [...list].sort((a, b) => {
      const aTime = a?.start_time ? new Date(a.start_time).getTime() : 0;
      const bTime = b?.start_time ? new Date(b.start_time).getTime() : 0;
      return aTime - bTime;
    });
    return sortedByTime.slice(sortedByTime.length - MAX_SUMMARY_ITEMS);
  }, [MAX_SUMMARY_ITEMS]);

  useEffect(() => {
    focusedRequestIdRef.current = focusedRequestId ?? null;
  }, [focusedRequestId]);

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
        // Invalid URL, fall through to default
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
            const snapshotData = Array.isArray(message.payload) ? message.payload : [];
            setSummaries(trimSummaries(snapshotData));
            break;

          case WS_MESSAGE_TYPES.NEW:
            // New request summary added
            setSummaries((prev) => trimSummaries([...prev, message.payload]));
            break;

          case WS_MESSAGE_TYPES.UPDATE:
            // Existing request summary updated
            setSummaries((prev) => {
              const updated = prev.map((r) =>
                r.id === message.payload.id ? message.payload : r
              );
              return trimSummaries(updated);
            });
            break;

          case WS_MESSAGE_TYPES.DELETE:
            // Request deleted (shouldn't happen often)
            setSummaries((prev) => {
              const filtered = prev.filter((r) => r.id !== message.payload.id);
              return trimSummaries(filtered);
            });
            break;

          case WS_MESSAGE_TYPES.CHANNEL:
            if (message.payload && message.payload.request_id) {
              const focusedId = focusedRequestIdRef.current;
              if (focusedId && message.payload.request_id === focusedId) {
                setChannelUpdate(message.payload);
              }
            }
            break;

          default:
          // Unknown message type, ignore
        }
      });
    } catch (error) {
      // Error parsing message, ignore
    }
  }, []);

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

    // Drop any stale in-memory live update when reconnecting.
    setChannelUpdate(null);

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
        }, STABLE_CONNECTION_TIMEOUT_MS);
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
    channelUpdate,
  };
};

export default useMonitorWs;
