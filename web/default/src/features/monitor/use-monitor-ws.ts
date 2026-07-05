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

import { useCallback, useEffect, useRef, useState } from 'react'

import { getMonitorStats } from './api'
import {
  MIN_SUMMARY_ITEMS,
  SUMMARY_RETENTION_WINDOW_MS,
  getStartTimeMs,
  normalizeMonitorPayload,
} from './lib'
import type {
  ChannelUpdate,
  MonitorRecord,
  MonitorStatsPayload,
  MonitorWsMessage,
} from './types'

const MAX_RECONNECT_ATTEMPTS = 10
const BASE_RECONNECT_DELAY_MS = 1000
const STABLE_CONNECTION_TIMEOUT_MS = 3000

const DEFAULT_STATS: MonitorStatsPayload = {
  total: 0,
  active: 0,
  memory: 0,
  load: {
    active_requests: 0,
    capacity: MIN_SUMMARY_ITEMS,
    degraded: false,
  },
}

function buildWsUrl(): string {
  const envBase = import.meta.env.VITE_REACT_APP_SERVER_URL as
    | string
    | undefined

  if (envBase) {
    try {
      const parsed = new URL(envBase)
      const protocol = parsed.protocol === 'https:' ? 'wss:' : 'ws:'
      const basePath = parsed.pathname.replace(/\/$/, '')
      return `${protocol}//${parsed.host}${basePath}/api/monitor/ws`
    } catch {
      // fall through
    }
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/monitor/ws`
}

function asMonitorRecord(payload: unknown): MonitorRecord | null {
  if (!payload || typeof payload !== 'object') return null
  const record = payload as Partial<MonitorRecord>
  return typeof record.id === 'string' ? (record as MonitorRecord) : null
}

function asChannelUpdate(payload: unknown): ChannelUpdate | null {
  if (!payload || typeof payload !== 'object') return null
  return payload as ChannelUpdate
}

function getRetryCountFromChannelUpdate(payload: ChannelUpdate): number {
  if (Array.isArray(payload.channel_attempts)) {
    return Math.max(0, payload.channel_attempts.length - 1)
  }

  const currentAttempt = Number(payload.current_channel?.attempt)
  if (Number.isFinite(currentAttempt) && currentAttempt > 1) {
    return currentAttempt - 1
  }

  return 0
}

function trimSummaries(list: MonitorRecord[]): MonitorRecord[] {
  if (list.length <= MIN_SUMMARY_ITEMS) return list

  const cutoffMs = Date.now() - SUMMARY_RETENTION_WINDOW_MS
  let removeCount = 0
  for (const record of list) {
    if (list.length - removeCount <= MIN_SUMMARY_ITEMS) break
    const startMs = getStartTimeMs(record)
    if (!startMs || startMs >= cutoffMs) break
    removeCount++
  }

  return removeCount > 0 ? list.slice(removeCount) : list
}

type UseMonitorWsOptions = {
  focusedRequestId?: string | null
}

export function useMonitorWs(options: UseMonitorWsOptions = {}) {
  const [summaries, setSummaries] = useState<MonitorRecord[]>([])
  const [stats, setStats] = useState<MonitorStatsPayload>(DEFAULT_STATS)
  const [connected, setConnected] = useState(false)
  const [channelUpdate, setChannelUpdate] = useState<ChannelUpdate | null>(null)

  const summariesRef = useRef<MonitorRecord[]>([])
  const pendingMessagesRef = useRef<MonitorWsMessage[]>([])
  const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const disconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const stableOpenTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const statsIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const reconnectAttemptsRef = useRef(0)
  const focusedRequestIdRef = useRef<string | null>(
    options.focusedRequestId ?? null
  )

  const fetchStats = useCallback(async () => {
    try {
      const response = await getMonitorStats()
      if (!response.success) return
      const data = response.data ?? {}
      const load = response.load ?? {}
      setStats({
        total: data.total_requests ?? 0,
        active: data.active_requests ?? 0,
        memory: data.memory_bytes ?? 0,
        load: {
          active_requests:
            load.active_requests ??
            data.active_requests ??
            DEFAULT_STATS.load.active_requests,
          capacity: load.capacity ?? DEFAULT_STATS.load.capacity,
          degraded: load.degraded ?? false,
        },
      })
    } catch {
      // stats are best-effort and will refresh on the next interval
    }
  }, [])

  const applyBatch = useCallback((batch: MonitorWsMessage[]) => {
    if (batch.length === 0) return

    let nextSummaries = summariesRef.current
    let changed = false
    let latestChannelUpdate: ChannelUpdate | null = null

    const ensureMutable = () => {
      if (nextSummaries === summariesRef.current) {
        nextSummaries = [...nextSummaries]
      }
    }

    batch.forEach((message) => {
      const receivedAtMs = Date.now()
      if (message.type === 'snapshot') {
        const payload = Array.isArray(message.payload) ? message.payload : []
        nextSummaries = payload
          .map(asMonitorRecord)
          .filter((record): record is MonitorRecord => record !== null)
          .map((record) => normalizeMonitorPayload(record, receivedAtMs))
        changed = true
        return
      }

      if (message.type === 'new' || message.type === 'update') {
        const record = asMonitorRecord(message.payload)
        if (!record) return
        const normalized = normalizeMonitorPayload(record, receivedAtMs)
        const existingIndex = nextSummaries.findIndex(
          (item) => item.id === normalized.id
        )
        ensureMutable()
        if (existingIndex === -1) {
          nextSummaries.push(normalized)
        } else {
          nextSummaries[existingIndex] = normalized
        }
        changed = true
        return
      }

      if (message.type === 'delete') {
        const record = asMonitorRecord(message.payload)
        if (!record) return
        if (nextSummaries.some((item) => item.id === record.id)) {
          nextSummaries = nextSummaries.filter((item) => item.id !== record.id)
          changed = true
        }
        return
      }

      if (message.type === 'channel_update') {
        const payload = asChannelUpdate(message.payload)
        if (!payload?.request_id) return
        const retryCount = getRetryCountFromChannelUpdate(payload)
        const existingIndex = nextSummaries.findIndex(
          (item) => item.id === payload.request_id
        )
        if (existingIndex !== -1) {
          ensureMutable()
          nextSummaries[existingIndex] = {
            ...nextSummaries[existingIndex],
            current_phase:
              payload.current_phase ??
              nextSummaries[existingIndex].current_phase,
            current_channel:
              payload.current_channel ??
              nextSummaries[existingIndex].current_channel,
            channel_attempts:
              payload.channel_attempts ??
              nextSummaries[existingIndex].channel_attempts,
            retry_count: retryCount,
            _receivedAtMs: receivedAtMs,
          }
          changed = true
        }

        if (focusedRequestIdRef.current === payload.request_id) {
          latestChannelUpdate = normalizeMonitorPayload(payload, receivedAtMs)
        }
      }
    })

    if (changed) {
      const trimmed = trimSummaries(nextSummaries)
      summariesRef.current = trimmed
      setSummaries(trimmed)
    }

    if (latestChannelUpdate) {
      setChannelUpdate(latestChannelUpdate)
    }
  }, [])

  const scheduleFlush = useCallback(() => {
    if (flushTimerRef.current) return
    flushTimerRef.current = setTimeout(() => {
      flushTimerRef.current = null
      const batch = pendingMessagesRef.current
      pendingMessagesRef.current = []
      applyBatch(batch)
    }, 50)
  }, [applyBatch])

  const handleMessage = useCallback(
    (event: MessageEvent) => {
      if (typeof event.data !== 'string') return
      const messages = event.data.split('\n').filter((item) => item.trim())
      messages.forEach((message) => {
        try {
          pendingMessagesRef.current.push(
            JSON.parse(message) as MonitorWsMessage
          )
        } catch {
          // ignore malformed websocket frames
        }
      })
      scheduleFlush()
    },
    [scheduleFlush]
  )

  const connect = useCallback(() => {
    if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
    reconnectTimeoutRef.current = null
    wsRef.current?.close()
    setChannelUpdate(null)
    pendingMessagesRef.current = []

    const ws = new WebSocket(buildWsUrl())
    ws.onopen = () => {
      if (disconnectTimerRef.current) clearTimeout(disconnectTimerRef.current)
      disconnectTimerRef.current = null
      setConnected(true)
      if (stableOpenTimerRef.current) clearTimeout(stableOpenTimerRef.current)
      stableOpenTimerRef.current = setTimeout(() => {
        reconnectAttemptsRef.current = 0
      }, STABLE_CONNECTION_TIMEOUT_MS)
    }
    ws.onmessage = handleMessage
    ws.onclose = () => {
      if (stableOpenTimerRef.current) clearTimeout(stableOpenTimerRef.current)
      stableOpenTimerRef.current = null
      if (disconnectTimerRef.current) clearTimeout(disconnectTimerRef.current)
      disconnectTimerRef.current = setTimeout(() => setConnected(false), 800)

      if (reconnectAttemptsRef.current < MAX_RECONNECT_ATTEMPTS) {
        const delay = Math.min(
          BASE_RECONNECT_DELAY_MS * 2 ** reconnectAttemptsRef.current,
          30000
        )
        reconnectTimeoutRef.current = setTimeout(() => {
          reconnectAttemptsRef.current += 1
          connect()
        }, delay)
      }
    }
    wsRef.current = ws
  }, [handleMessage])

  const reconnect = useCallback(() => {
    reconnectAttemptsRef.current = 0
    connect()
  }, [connect])

  useEffect(() => {
    focusedRequestIdRef.current = options.focusedRequestId ?? null
  }, [options.focusedRequestId])

  useEffect(() => {
    connect()
    return () => {
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
      if (disconnectTimerRef.current) clearTimeout(disconnectTimerRef.current)
      if (stableOpenTimerRef.current) clearTimeout(stableOpenTimerRef.current)
      if (flushTimerRef.current) clearTimeout(flushTimerRef.current)
      wsRef.current?.close()
    }
  }, [connect])

  useEffect(() => {
    void fetchStats()
    statsIntervalRef.current = setInterval(fetchStats, 2000)
    return () => {
      if (statsIntervalRef.current) clearInterval(statsIntervalRef.current)
    }
  }, [fetchStats])

  return { summaries, stats, connected, reconnect, channelUpdate }
}
