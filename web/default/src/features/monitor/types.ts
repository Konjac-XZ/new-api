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

export type MonitorStatus =
  | 'pending'
  | 'processing'
  | 'waiting_upstream'
  | 'streaming'
  | 'completed'
  | 'error'
  | 'abandoned'
  | string

export type MonitorBodyType = 'downstream' | 'upstream' | 'response'

export type MonitorBodyResponse = {
  success: boolean
  message?: string
  type?: MonitorBodyType
  body?: string
  size?: number
  truncated?: boolean
}

export type MonitorStats = {
  total_requests?: number
  active_requests?: number
  completed?: number
  errors?: number
  abandoned?: number
  memory_bytes?: number
}

export type MonitorLoad = {
  active_requests?: number
  capacity?: number
  degraded?: boolean
}

export type MonitorStatsPayload = {
  total: number
  active: number
  memory: number
  load: Required<MonitorLoad>
}

export type CurrentChannel = {
  id?: number
  name?: string
  attempt?: number
}

export type ChannelAttempt = {
  attempt?: number
  channel_id?: number
  channel_name?: string
  started_at?: string
  started_at_ms?: number
  streaming_started_at?: string
  streaming_started_at_ms?: number
  ended_at?: string
  ended_at_ms?: number
  status?: string
  reason?: string
  error_code?: string
  http_status?: number
}

export type MonitorErrorInfo = {
  code?: string
  message?: string
}

export type MonitorDownstreamInfo = {
  method?: string
  path?: string
  headers?: Record<string, string>
  body?: string
  body_size?: number
  body_truncated?: boolean
  client_ip?: string
}

export type MonitorUpstreamInfo = {
  url?: string
  method?: string
  headers?: Record<string, string>
  body?: string
  body_size?: number
  body_truncated?: boolean
}

export type MonitorResponseInfo = {
  status_code?: number
  headers?: Record<string, string>
  body?: string
  body_size?: number
  body_truncated?: boolean
  error?: MonitorErrorInfo
  prompt_tokens?: number
  completion_tokens?: number
}

export type MonitorRecord = {
  id: string
  status?: MonitorStatus
  server_now_ms?: number
  start_time?: string
  start_time_ms?: number
  end_time?: string
  end_time_ms?: number
  duration_ms?: number
  downstream?: MonitorDownstreamInfo
  upstream?: MonitorUpstreamInfo
  response?: MonitorResponseInfo
  user_id?: number
  token_id?: number
  token_name?: string
  channel_id?: number
  channel_name?: string
  model?: string
  upstream_model?: string
  is_model_mapped?: boolean
  is_stream?: boolean
  prompt_tokens?: number
  completion_tokens?: number
  current_phase?: string
  current_channel?: CurrentChannel
  channel_attempts?: ChannelAttempt[]
  retry_count?: number
  current_attempt_started_at_ms?: number
  current_attempt_streaming_started_at_ms?: number
  status_code?: number
  has_error?: boolean
  _receivedAtMs?: number
}

export type ChannelUpdate = {
  request_id?: string
  server_now_ms?: number
  current_phase?: string
  current_channel?: CurrentChannel
  channel_attempts?: ChannelAttempt[]
}

export type MonitorWsMessage = {
  type?: 'new' | 'update' | 'delete' | 'snapshot' | 'channel_update' | string
  payload?: unknown
}

export type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data?: T
}
