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

import { api } from '@/lib/api'

import type {
  ApiEnvelope,
  MonitorBodyResponse,
  MonitorBodyType,
  MonitorLoad,
  MonitorRecord,
  MonitorStats,
} from './types'

export async function getMonitorStats(): Promise<{
  success: boolean
  data?: MonitorStats
  load?: MonitorLoad
}> {
  const res = await api.get('/api/monitor/stats', {
    skipErrorHandler: true,
    disableDuplicate: true,
  })
  return res.data
}

export async function getMonitorRequest(
  id: string
): Promise<ApiEnvelope<MonitorRecord>> {
  const res = await api.get(`/api/monitor/requests/${id}`, {
    skipErrorHandler: true,
    disableDuplicate: true,
  })
  return res.data
}

export async function getMonitorBody(
  id: string,
  type: MonitorBodyType
): Promise<MonitorBodyResponse> {
  const res = await api.get(`/api/monitor/requests/${id}/body/${type}`, {
    skipErrorHandler: true,
    disableDuplicate: true,
  })
  return res.data
}

export async function interruptMonitorRequest(
  id: string
): Promise<ApiEnvelope<unknown>> {
  const res = await api.post(
    `/api/monitor/requests/${id}/interrupt`,
    {},
    { skipErrorHandler: true }
  )
  return res.data
}
