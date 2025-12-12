import { useState, useCallback, useRef } from 'react';
import { API } from '../../helpers/api';

const useRequestDetail = () => {
  const [selectedDetail, setSelectedDetail] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [interrupting, setInterrupting] = useState(false);
  // Cache for fetched details: Map<id, RequestRecord>
  const cacheRef = useRef(new Map());

  // Track which IDs are currently being fetched to prevent duplicate requests
  const fetchingRef = useRef(new Set());

  const fetchDetail = useCallback(async (id) => {
    if (!id) {
      setSelectedDetail(null);
      setError(null);
      return;
    }

    // Check cache first
    if (cacheRef.current.has(id)) {
      setSelectedDetail(cacheRef.current.get(id));
      setError(null);
      return;
    }

    // Prevent duplicate fetches
    if (fetchingRef.current.has(id)) {
      return;
    }

    fetchingRef.current.add(id);
    setLoading(true);
    setError(null);

    try {
      const response = await API.get(`/api/monitor/requests/${id}`, {
        skipErrorHandler: true,
        disableDuplicate: true,
      });
      if (response.data.success) {
        const detail = response.data.data;
        cacheRef.current.set(id, detail);
        setSelectedDetail(detail);
      } else {
        setError(response.data.message || 'Failed to fetch request details');
      }
    } catch (err) {
      if (err.response?.status === 404) {
        setError('Request not found (may have been evicted from buffer)');
      } else {
        setError(err.message || 'Failed to fetch request details');
      }
      console.error('Error fetching request detail:', err);
    } finally {
      setLoading(false);
      fetchingRef.current.delete(id);
    }
  }, []);

  // Invalidate cache entry (e.g., when status changes from processing to completed)
  const invalidateCache = useCallback((id) => {
    cacheRef.current.delete(id);
  }, []);

  // Apply real-time partial updates (e.g., channel_update over WebSocket)
  const applyLiveUpdate = useCallback((id, patch) => {
    if (!id || !patch) return;

    const normalizedPatch = { ...patch };
    if (patch.request_id && !normalizedPatch.id) {
      normalizedPatch.id = patch.request_id;
    }

    setSelectedDetail((prev) => {
      if (!prev || prev.id !== id) return prev;
      return {
        ...prev,
        ...normalizedPatch,
        channel_attempts: normalizedPatch.channel_attempts || prev.channel_attempts,
        current_channel: normalizedPatch.current_channel || prev.current_channel,
        current_phase: normalizedPatch.current_phase || prev.current_phase,
      };
    });

    const existing = cacheRef.current.get(id) || {};
    cacheRef.current.set(id, {
      ...existing,
      ...normalizedPatch,
      channel_attempts: normalizedPatch.channel_attempts || existing.channel_attempts,
      current_channel: normalizedPatch.current_channel || existing.current_channel,
      current_phase: normalizedPatch.current_phase || existing.current_phase,
    });
  }, []);

  // Clear entire cache (e.g., on reconnect)
  const clearCache = useCallback(({ preserveSelection = false } = {}) => {
    cacheRef.current.clear();
    if (!preserveSelection) {
      setSelectedDetail(null);
    }
    setError(null);
  }, []);

  // Interrupt an active request
  const interruptRequest = useCallback(async (id) => {
    if (!id) return { success: false, error: 'No request ID provided' };

    setInterrupting(true);
    try {
      const response = await API.post(`/api/monitor/requests/${id}/interrupt`, {}, {
        skipErrorHandler: true,
      });

      if (response.data.success) {
        // Invalidate cache to force refetch with updated state
        invalidateCache(id);

        // Refetch if this is the currently selected request
        if (selectedDetail?.id === id) {
          await fetchDetail(id);
        }

        return { success: true };
      } else {
        return { success: false, error: response.data.message || 'Failed to interrupt request' };
      }
    } catch (err) {
      const errorMsg = err.response?.data?.message || err.message || 'Failed to interrupt request';
      return { success: false, error: errorMsg };
    } finally {
      setInterrupting(false);
    }
  }, [invalidateCache, fetchDetail, selectedDetail]);

  return {
    selectedDetail,
    loading,
    error,
    interrupting,
    fetchDetail,
    invalidateCache,
    clearCache,
    applyLiveUpdate,
    interruptRequest,
  };
};

export default useRequestDetail;
