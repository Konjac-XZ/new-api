/*
Copyright (C) 2023-2026 QuantumNous

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

import { useCallback, useRef, useState } from 'react'

import { getMonitorRequest, interruptMonitorRequest } from './api'
import { normalizeMonitorPayload } from './lib'
import type { ChannelUpdate, MonitorRecord } from './types'

const MAX_DETAIL_CACHE_SIZE = 50

export function useRequestDetail() {
  const [selectedDetail, setSelectedDetail] = useState<MonitorRecord | null>(
    null
  )
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [interrupting, setInterrupting] = useState(false)

  const cacheRef = useRef(new Map<string, MonitorRecord>())
  const fetchingRef = useRef(new Set<string>())
  const latestRequestIdRef = useRef<string | null>(null)

  const fetchDetail = useCallback(async (id: string | null) => {
    if (!id) {
      latestRequestIdRef.current = null
      setLoading(false)
      setSelectedDetail(null)
      setError(null)
      return
    }

    latestRequestIdRef.current = id
    const cached = cacheRef.current.get(id)
    if (cached) {
      cacheRef.current.delete(id)
      cacheRef.current.set(id, cached)
      setSelectedDetail(cached)
      setError(null)
      return
    }

    if (fetchingRef.current.has(id)) return
    fetchingRef.current.add(id)
    setLoading(true)
    setError(null)

    try {
      const response = await getMonitorRequest(id)
      if (response.success && response.data) {
        const detail = normalizeMonitorPayload(response.data)
        cacheRef.current.set(id, detail)
        if (cacheRef.current.size > MAX_DETAIL_CACHE_SIZE) {
          const oldestKey = cacheRef.current.keys().next().value
          if (oldestKey) cacheRef.current.delete(oldestKey)
        }
        if (latestRequestIdRef.current === id) setSelectedDetail(detail)
      } else if (latestRequestIdRef.current === id) {
        setError(response.message || 'Request failed')
      }
    } catch (err) {
      if (latestRequestIdRef.current === id) {
        setError(err instanceof Error ? err.message : 'Request failed')
      }
    } finally {
      if (latestRequestIdRef.current === id) setLoading(false)
      fetchingRef.current.delete(id)
    }
  }, [])

  const invalidateCache = useCallback((id: string) => {
    cacheRef.current.delete(id)
  }, [])

  const applyLiveUpdate = useCallback((id: string, patch: ChannelUpdate) => {
    if (!id) return
    const normalized = normalizeMonitorPayload({
      ...patch,
      id: patch.request_id ?? id,
    } as MonitorRecord)

    setSelectedDetail((prev) => {
      if (!prev || prev.id !== id) return prev
      return {
        ...prev,
        ...normalized,
        channel_attempts: normalized.channel_attempts ?? prev.channel_attempts,
        current_channel: normalized.current_channel ?? prev.current_channel,
        current_phase: normalized.current_phase ?? prev.current_phase,
      }
    })

    const existing = cacheRef.current.get(id)
    if (existing) {
      cacheRef.current.set(id, {
        ...existing,
        ...normalized,
        channel_attempts:
          normalized.channel_attempts ?? existing.channel_attempts,
        current_channel: normalized.current_channel ?? existing.current_channel,
        current_phase: normalized.current_phase ?? existing.current_phase,
      })
    }
  }, [])

  const interruptRequest = useCallback(
    async (id: string) => {
      setInterrupting(true)
      try {
        const response = await interruptMonitorRequest(id)
        if (response.success) {
          invalidateCache(id)
          if (selectedDetail?.id === id) await fetchDetail(id)
          return { success: true, error: null }
        }
        return { success: false, error: response.message || 'Request failed' }
      } catch (err) {
        return {
          success: false,
          error: err instanceof Error ? err.message : 'Request failed',
        }
      } finally {
        setInterrupting(false)
      }
    },
    [fetchDetail, invalidateCache, selectedDetail?.id]
  )

  return {
    selectedDetail,
    loading,
    error,
    interrupting,
    fetchDetail,
    invalidateCache,
    applyLiveUpdate,
    interruptRequest,
  }
}
