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
export type ConversationRole =
  | 'assistant'
  | 'developer'
  | 'system'
  | 'tool'
  | 'unknown'
  | 'user'

export type ConversationPart = {
  kind: 'image' | 'json' | 'text' | 'thinking'
  label?: string
  text: string
}

export type ConversationMessage = {
  name?: string
  originalRole: string
  parts: ConversationPart[]
  role: ConversationRole
}

export type ConversationBody = {
  messages: ConversationMessage[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function prettyValue(value: unknown): string {
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  }

  try {
    const serialized = JSON.stringify(value, null, 2)
    return serialized ?? String(value)
  } catch {
    return String(value)
  }
}

function getRole(role: string): ConversationRole {
  if (
    role === 'assistant' ||
    role === 'developer' ||
    role === 'system' ||
    role === 'tool' ||
    role === 'user'
  ) {
    return role
  }
  return 'unknown'
}

function describeMedia(block: Record<string, unknown>): string {
  const source = isRecord(block.source) ? block.source : undefined
  const imageUrl = isRecord(block.image_url)
    ? block.image_url.url
    : block.image_url
  const reference =
    source?.url ?? source?.media_type ?? source?.type ?? imageUrl
  if (typeof reference !== 'string' || !reference) return ''
  return reference.length > 240 ? `${reference.slice(0, 240)}…` : reference
}

function parseContent(value: unknown): ConversationPart[] {
  if (typeof value === 'string') {
    return [{ kind: 'text', text: value }]
  }
  if (value === null || value === undefined) return []
  if (!Array.isArray(value)) {
    return [{ kind: 'json', text: prettyValue(value) }]
  }

  const parts: ConversationPart[] = []
  for (const item of value) {
    if (typeof item === 'string') {
      parts.push({ kind: 'text', text: item })
      continue
    }
    if (!isRecord(item)) {
      parts.push({ kind: 'json', text: prettyValue(item) })
      continue
    }

    const type = typeof item.type === 'string' ? item.type : ''
    if (
      (type === 'text' || type === 'input_text' || type === 'output_text') &&
      typeof item.text === 'string'
    ) {
      parts.push({ kind: 'text', text: item.text })
      continue
    }
    if (type === 'thinking' && typeof item.thinking === 'string') {
      parts.push({ kind: 'thinking', text: item.thinking })
      continue
    }
    if (
      type === 'image' ||
      type === 'image_url' ||
      type === 'input_image' ||
      type === 'document'
    ) {
      parts.push({
        kind: 'image',
        label: type === 'document' ? 'Document' : 'Image',
        text: describeMedia(item),
      })
      continue
    }
    if (type === 'tool_result') {
      const toolParts = parseContent(item.content)
      parts.push(
        ...toolParts.map((part) => ({
          ...part,
          label:
            typeof item.tool_use_id === 'string'
              ? item.tool_use_id
              : part.label,
        }))
      )
      continue
    }
    if (type === 'tool_use') {
      parts.push({
        kind: 'json',
        label: typeof item.name === 'string' ? item.name : undefined,
        text: prettyValue(item.input),
      })
      continue
    }

    parts.push({
      kind: 'json',
      label: type || undefined,
      text: prettyValue(item),
    })
  }
  return parts
}

function parseMessage(value: unknown): ConversationMessage | null {
  if (!isRecord(value) || typeof value.role !== 'string') return null

  const parts = parseContent(value.content)
  const toolCalls = Array.isArray(value.tool_calls) ? value.tool_calls : []
  for (const toolCall of toolCalls) {
    if (!isRecord(toolCall)) continue
    const fn = isRecord(toolCall.function) ? toolCall.function : toolCall
    parts.push({
      kind: 'json',
      label: typeof fn.name === 'string' ? fn.name : undefined,
      text: prettyValue(fn.arguments ?? fn.input ?? toolCall),
    })
  }
  if (isRecord(value.function_call)) {
    parts.push({
      kind: 'json',
      label:
        typeof value.function_call.name === 'string'
          ? value.function_call.name
          : undefined,
      text: prettyValue(value.function_call.arguments),
    })
  }

  return {
    name: typeof value.name === 'string' ? value.name : undefined,
    originalRole: value.role,
    parts,
    role: getRole(value.role),
  }
}

export function parseConversationBody(body: string): ConversationBody | null {
  let payload: unknown
  try {
    payload = JSON.parse(body)
    if (typeof payload === 'string') payload = JSON.parse(payload)
  } catch {
    return null
  }
  if (!isRecord(payload) || !Array.isArray(payload.messages)) return null

  const messages: ConversationMessage[] = []
  if (payload.system !== undefined) {
    const parts = parseContent(payload.system)
    if (parts.length > 0) {
      messages.push({ originalRole: 'system', parts, role: 'system' })
    }
  }
  for (const value of payload.messages) {
    const message = parseMessage(value)
    if (message) messages.push(message)
  }

  return messages.length > 0 ? { messages } : null
}
