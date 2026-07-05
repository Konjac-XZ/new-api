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

import type { MonitorRecord } from './types'

export const MIN_SUMMARY_ITEMS = 100
export const SUMMARY_RETENTION_WINDOW_MS = 5 * 60 * 1000
export const MS_TO_SECONDS = 1000
export const MIN_OUTPUT_TOKENS_FOR_THROUGHPUT = 100

const ACTIVE_STATUSES = new Set(['processing', 'waiting_upstream', 'streaming'])
const TERMINAL_STATUSES = new Set(['completed', 'error', 'abandoned'])

export function deriveDisplayStatus(record: MonitorRecord | null): string {
  if (!record) return ''

  const status = record.status || ''
  const currentPhase = record.current_phase || ''

  if (status === 'completed' || status === 'error') return status
  if (status === 'waiting_upstream' || status === 'streaming') return status
  if (
    (status === 'processing' || status === 'pending') &&
    (currentPhase === 'streaming' || currentPhase === 'waiting_upstream')
  ) {
    return currentPhase
  }
  if (currentPhase === 'streaming' || currentPhase === 'waiting_upstream') {
    return currentPhase
  }

  return status
}

export function isActiveStatus(status: string): boolean {
  return ACTIVE_STATUSES.has(status)
}

export function isTerminalStatus(status: string): boolean {
  return TERMINAL_STATUSES.has(status)
}

export function getStartTimeMs(record: MonitorRecord): number {
  if (
    Number.isFinite(record.start_time_ms) &&
    Number(record.start_time_ms) > 0
  ) {
    return Number(record.start_time_ms)
  }

  const parsed = record.start_time ? new Date(record.start_time).getTime() : 0
  return Number.isFinite(parsed) ? parsed : 0
}

export function getSyncedNowMs(
  record: MonitorRecord,
  clientNowMs: number
): number {
  if (!record.server_now_ms || !record._receivedAtMs) return clientNowMs
  return record.server_now_ms + (clientNowMs - record._receivedAtMs)
}

export function getDurationMs(
  record: MonitorRecord,
  clientNowMs: number
): number {
  if (record.duration_ms && record.duration_ms > 0) return record.duration_ms
  const displayStatus = deriveDisplayStatus(record)
  if (!isActiveStatus(displayStatus)) return 0
  const startTimeMs = getStartTimeMs(record)
  if (!startTimeMs) return 0
  return Math.max(0, getSyncedNowMs(record, clientNowMs) - startTimeMs)
}

export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '-'
  return `${(ms / MS_TO_SECONDS).toFixed(1)}s`
}

export function formatDateTime(value?: string, valueMs?: number): string {
  let date: Date | null = null
  if (valueMs && valueMs > 0) {
    date = new Date(valueMs)
  } else if (value) {
    date = new Date(value)
  }
  if (!date || Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

export function getOptionalTokenCount(value: unknown): number | null {
  if (value === undefined || value === null || value === '') return null
  const tokenCount = Number(value)
  if (!Number.isFinite(tokenCount) || tokenCount < 0) return null
  return Math.floor(tokenCount)
}

export function isSuccessfulTokenResponse(record: MonitorRecord): boolean {
  if (record.status !== 'completed') return false
  if (record.has_error || record.response?.error) return false
  const statusCode = Number(record.response?.status_code ?? record.status_code)
  return !Number.isFinite(statusCode) || statusCode <= 0 || statusCode < 400
}

export function getMonitorTokenUsage(record: MonitorRecord): {
  promptTokens: number
  completionTokens: number | null
} {
  const promptTokens =
    getOptionalTokenCount(record.response?.prompt_tokens) ??
    getOptionalTokenCount(record.prompt_tokens) ??
    0

  if (!isSuccessfulTokenResponse(record)) {
    return { promptTokens, completionTokens: null }
  }

  return {
    promptTokens,
    completionTokens:
      getOptionalTokenCount(record.response?.completion_tokens) ??
      getOptionalTokenCount(record.completion_tokens) ??
      0,
  }
}

export function formatTokenCount(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return '-'
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K`
  return String(value)
}

export function getTtftMs(record: MonitorRecord): number | null {
  if (!record.is_stream) return null
  const streamingStartedAt = record.current_attempt_streaming_started_at_ms
  const attemptStartedAt = record.current_attempt_started_at_ms
  if (
    !streamingStartedAt ||
    !attemptStartedAt ||
    streamingStartedAt <= attemptStartedAt
  ) {
    return null
  }
  return streamingStartedAt - attemptStartedAt
}

export function getOutputSpeed(
  record: MonitorRecord,
  clientNowMs: number
): number | null {
  if (!record.is_stream) return null
  const completionTokens = getOptionalTokenCount(record.completion_tokens)
  if (
    !completionTokens ||
    completionTokens < MIN_OUTPUT_TOKENS_FOR_THROUGHPUT
  ) {
    return null
  }

  const startedAt = record.current_attempt_streaming_started_at_ms
  if (!startedAt) return null

  const endTimeMs = record.end_time_ms || 0
  const generationMs =
    endTimeMs > startedAt
      ? endTimeMs - startedAt
      : getSyncedNowMs(record, clientNowMs) - startedAt

  if (generationMs <= 0) return null
  return completionTokens / (generationMs / MS_TO_SECONDS)
}

export function getRetryCount(record: MonitorRecord): number {
  if (typeof record.retry_count === 'number') {
    return Math.max(0, record.retry_count)
  }
  const currentAttempt = Number(record.current_channel?.attempt)
  if (Number.isFinite(currentAttempt) && currentAttempt > 1) {
    return currentAttempt - 1
  }
  if (Array.isArray(record.channel_attempts)) {
    return Math.max(0, record.channel_attempts.length - 1)
  }
  return 0
}

export function normalizeMonitorPayload<T extends object>(
  payload: T,
  receivedAtMs = Date.now()
): T & { _receivedAtMs: number } {
  return { ...payload, _receivedAtMs: receivedAtMs }
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}
