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
import type { ChannelSortBy, ChannelSortOrder, ChannelSortRule } from '../types'

export const CHANNEL_SORT_RULES_STORAGE_KEY = 'channel-sort-rules:v1'
export const DEFAULT_CHANNEL_SORT_RULES: ChannelSortRule[] = [
  { field: 'priority', order: 'desc' },
]
export const EXAMPLE_CHANNEL_SORT_RULES: ChannelSortRule[] = [
  { field: 'priority', order: 'desc' },
  { field: 'weight', order: 'desc' },
  { field: 'name', order: 'asc' },
  { field: 'id', order: 'desc' },
]

export const CHANNEL_SORT_FIELDS: ChannelSortBy[] = [
  'priority',
  'weight',
  'name',
  'id',
  'balance',
  'response_time',
  'test_time',
]

export const CHANNEL_SORT_FIELD_LABELS: Record<ChannelSortBy, string> = {
  priority: 'Priority',
  weight: 'Weight',
  name: 'Name',
  id: 'ID',
  balance: 'Balance',
  response_time: 'Response Time',
  test_time: 'Last Tested',
}

export const CHANNEL_SORT_ORDER_LABELS: Record<ChannelSortOrder, string> = {
  desc: 'Desc',
  asc: 'Asc',
}

const CHANNEL_SORT_FIELD_SET = new Set<ChannelSortBy>(CHANNEL_SORT_FIELDS)
const CHANNEL_SORT_ORDER_SET = new Set<ChannelSortOrder>(['asc', 'desc'])

export function normalizeChannelSortRules(rules: unknown): ChannelSortRule[] {
  if (!Array.isArray(rules)) {
    return [...DEFAULT_CHANNEL_SORT_RULES]
  }

  const normalized: ChannelSortRule[] = []
  const seen = new Set<ChannelSortBy>()

  for (const rule of rules) {
    if (!rule || typeof rule !== 'object') continue

    const rawRule = rule as Record<string, unknown>
    const field = String(rawRule.field || '')
      .trim()
      .toLowerCase() as ChannelSortBy
    if (!CHANNEL_SORT_FIELD_SET.has(field) || seen.has(field)) continue

    const order = CHANNEL_SORT_ORDER_SET.has(rawRule.order as ChannelSortOrder)
      ? (rawRule.order as ChannelSortOrder)
      : 'desc'

    seen.add(field)
    normalized.push({ field, order })
  }

  return normalized.length > 0 ? normalized : [...DEFAULT_CHANNEL_SORT_RULES]
}

export function loadStoredChannelSortRules(): ChannelSortRule[] {
  try {
    const storedRules = localStorage.getItem(CHANNEL_SORT_RULES_STORAGE_KEY)
    if (storedRules) {
      return normalizeChannelSortRules(JSON.parse(storedRules))
    }

    if (
      localStorage.getItem('channels-id-sort') === 'true' ||
      localStorage.getItem('id-sort') === 'true'
    ) {
      return [{ field: 'id', order: 'desc' }]
    }
  } catch {
    // Fall through to default preferences if local storage is unavailable.
  }

  return [...DEFAULT_CHANNEL_SORT_RULES]
}

export function persistChannelSortRules(rules: ChannelSortRule[]): void {
  try {
    localStorage.setItem(
      CHANNEL_SORT_RULES_STORAGE_KEY,
      JSON.stringify(normalizeChannelSortRules(rules))
    )
  } catch {
    // Keep the in-memory state when local storage is unavailable.
  }
}

export function serializeChannelSortRules(rules: ChannelSortRule[]): string {
  return JSON.stringify(normalizeChannelSortRules(rules))
}

export function getChannelSortSummary(
  rules: ChannelSortRule[],
  t: (key: string) => string
): string {
  return normalizeChannelSortRules(rules)
    .map(
      (rule) =>
        `${t(CHANNEL_SORT_FIELD_LABELS[rule.field])} ${t(
          CHANNEL_SORT_ORDER_LABELS[rule.order]
        )}`
    )
    .join(' / ')
}
