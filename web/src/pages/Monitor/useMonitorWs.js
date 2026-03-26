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

const getTimestampMs = (msValue, fallbackValue) => {
  if (Number.isFinite(msValue) && msValue > 0) {
    return msValue;
  }

  if (!fallbackValue) {
    return 0;
  }

  const parsed = new Date(fallbackValue).getTime();
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
};

const useMonitorWs = ({ focusedRequestId } = {}) => {
  const [summaries, setSummaries] = useState([]);
  const [stats, setStats] = useState({ total: 0, active: 0, memory: 0 });
  const [connected, setConnected] = useState(false);
  // Only keep the most recent channel update for the currently focused request
  // to avoid unbounded memory growth when the page is open for long periods.
  const [channelUpdate, setChannelUpdate] = useState(null);
  const summariesRef = useRef([]);
  const pendingMessagesRef = useRef([]);
  const flushTimerRef = useRef(null);
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
      const aTime = getTimestampMs(a?.start_time_ms, a?.start_time);
      const bTime = getTimestampMs(b?.start_time_ms, b?.start_time);
      return aTime - bTime;
    });
    return sortedByTime.slice(sortedByTime.length - MAX_SUMMARY_ITEMS);
  }, [MAX_SUMMARY_ITEMS]);

  const applyBatch = useCallback((batch) => {
    if (!batch.length) return;

    let nextSummaries = summariesRef.current;
    let changed = false;
    let latestChannelUpdate = null;

    const ensureMutable = () => {
      if (nextSummaries === summariesRef.current) {
        nextSummaries = [...nextSummaries];
      }
    };

    batch.forEach((message) => {
      switch (message.type) {
        case WS_MESSAGE_TYPES.SNAPSHOT: {
          const snapshotData = Array.isArray(message.payload) ? message.payload : [];
          nextSummaries = snapshotData;
          changed = true;
          break;
        }
        case WS_MESSAGE_TYPES.NEW: {
          if (!message.payload) break;
          const existingIndex = nextSummaries.findIndex((item) => item.id === message.payload.id);
          if (existingIndex === -1) {
            ensureMutable();
            nextSummaries.push(message.payload);
            changed = true;
          } else if (nextSummaries[existingIndex] !== message.payload) {
            ensureMutable();
            nextSummaries[existingIndex] = message.payload;
            changed = true;
          }
          break;
        }
        case WS_MESSAGE_TYPES.UPDATE: {
          if (!message.payload) break;
          const existingIndex = nextSummaries.findIndex((item) => item.id === message.payload.id);
          if (existingIndex !== -1) {
            ensureMutable();
            nextSummaries[existingIndex] = message.payload;
            changed = true;
          } else {
            ensureMutable();
            nextSummaries.push(message.payload);
            changed = true;
          }
          break;
        }
        case WS_MESSAGE_TYPES.DELETE: {
          if (!message.payload) break;
          const existingIndex = nextSummaries.findIndex((item) => item.id === message.payload.id);
          if (existingIndex !== -1) {
            ensureMutable();
            nextSummaries = nextSummaries.filter((item) => item.id !== message.payload.id);
            changed = true;
          }
          break;
        }
        case WS_MESSAGE_TYPES.CHANNEL: {
          if (message.payload && message.payload.request_id) {
            const focusedId = focusedRequestIdRef.current;
            if (focusedId && message.payload.request_id === focusedId) {
              latestChannelUpdate = message.payload;
            }
          }
          break;
        }
        default:
          break;
      }
    });

    if (changed) {
      const trimmed = trimSummaries(nextSummaries);
      summariesRef.current = trimmed;
      setSummaries(trimmed);
    }

    if (latestChannelUpdate) {
      setChannelUpdate(latestChannelUpdate);
    }
  }, [trimSummaries]);

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

  const scheduleFlush = useCallback(() => {
    if (flushTimerRef.current) return;
    flushTimerRef.current = setTimeout(() => {
      flushTimerRef.current = null;
      const batch = pendingMessagesRef.current;
      if (batch.length === 0) return;
      pendingMessagesRef.current = [];
      applyBatch(batch);
    }, 50);
  }, [applyBatch]);

  const handleMessage = useCallback((event) => {
    try {
      if (typeof event.data !== 'string') return;
      const messages = event.data.split('\n').filter((m) => m.trim());
      if (messages.length === 0) return;

      messages.forEach((msgStr) => {
        const message = JSON.parse(msgStr);
        pendingMessagesRef.current.push(message);
      });

      scheduleFlush();
    } catch (error) {
      // Error parsing message, ignore
    }
  }, [scheduleFlush]);

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
    pendingMessagesRef.current = [];
    if (flushTimerRef.current) {
      clearTimeout(flushTimerRef.current);
      flushTimerRef.current = null;
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
      if (flushTimerRef.current) {
        clearTimeout(flushTimerRef.current);
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
