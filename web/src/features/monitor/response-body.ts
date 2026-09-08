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
export type ResponseBodyPart = {
  kind: 'content' | 'thinking'
  text: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function appendPart(
  parts: ResponseBodyPart[],
  kind: ResponseBodyPart['kind'],
  text: unknown
) {
  if (typeof text !== 'string' || text === '') return
  const previous = parts.at(-1)
  if (previous?.kind === kind) {
    previous.text += text
    return
  }
  parts.push({ kind, text })
}

function splitThinkingTags(text: string): ResponseBodyPart[] {
  const parts: ResponseBodyPart[] = []
  const tagPattern = /<\s*(\/?)\s*think(?:ing)?\s*>/gi
  let thinking = false
  let cursor = 0
  let match: RegExpExecArray | null

  while ((match = tagPattern.exec(text)) !== null) {
    appendPart(
      parts,
      thinking ? 'thinking' : 'content',
      text.slice(cursor, match.index)
    )
    thinking = match[1] !== '/'
    cursor = tagPattern.lastIndex
  }
  appendPart(parts, thinking ? 'thinking' : 'content', text.slice(cursor))
  return parts
}

function contentText(value: unknown): string {
  if (typeof value === 'string') return value
  if (!Array.isArray(value)) return ''

  let text = ''
  for (const block of value) {
    if (!isRecord(block)) continue
    if (
      block.type === 'text' ||
      block.type === 'input_text' ||
      block.type === 'output_text'
    ) {
      if (typeof block.text === 'string') text += block.text
    }
  }
  return text
}

function parseOpenAIResponse(
  payload: Record<string, unknown>
): ResponseBodyPart[] {
  if (!Array.isArray(payload.choices)) return []
  const parts: ResponseBodyPart[] = []
  for (const choice of payload.choices) {
    if (!isRecord(choice)) continue
    let message: Record<string, unknown> | undefined
    if (isRecord(choice.message)) {
      message = choice.message
    } else if (isRecord(choice.delta)) {
      message = choice.delta
    }
    if (!message) continue
    appendPart(
      parts,
      'thinking',
      message.reasoning_content ?? message.reasoning
    )
    const text = contentText(message.content)
    for (const part of splitThinkingTags(text)) {
      appendPart(parts, part.kind, part.text)
    }
  }
  return parts
}

function parseAnthropicResponse(
  payload: Record<string, unknown>
): ResponseBodyPart[] {
  if (!Array.isArray(payload.content)) return []
  const parts: ResponseBodyPart[] = []
  for (const block of payload.content) {
    if (!isRecord(block)) continue
    if (block.type === 'thinking') {
      appendPart(parts, 'thinking', block.thinking)
      continue
    }
    if (block.type === 'text' && typeof block.text === 'string') {
      for (const part of splitThinkingTags(block.text)) {
        appendPart(parts, part.kind, part.text)
      }
    }
  }
  return parts
}

function parseGeminiResponse(
  payload: Record<string, unknown>
): ResponseBodyPart[] {
  if (!Array.isArray(payload.candidates)) return []
  const parts: ResponseBodyPart[] = []
  for (const candidate of payload.candidates) {
    if (!isRecord(candidate) || !isRecord(candidate.content)) continue
    const blocks = candidate.content.parts
    if (!Array.isArray(blocks)) continue
    for (const block of blocks) {
      if (!isRecord(block) || typeof block.text !== 'string') continue
      appendPart(
        parts,
        block.thought === true ? 'thinking' : 'content',
        block.text
      )
    }
  }
  return parts
}

function parseResponsesApiResponse(
  payload: Record<string, unknown>
): ResponseBodyPart[] {
  if (!Array.isArray(payload.output)) return []
  const parts: ResponseBodyPart[] = []
  for (const output of payload.output) {
    if (!isRecord(output)) continue
    if (output.type === 'reasoning') {
      const summary = Array.isArray(output.summary) ? output.summary : []
      for (const item of summary) {
        if (isRecord(item)) appendPart(parts, 'thinking', item.text)
      }
      continue
    }
    if (output.type === 'message') {
      const text = contentText(output.content)
      for (const part of splitThinkingTags(text)) {
        appendPart(parts, part.kind, part.text)
      }
    }
  }
  return parts
}

export function parseResponseBody(body: string): ResponseBodyPart[] | null {
  const trimmed = body.trim()
  if (!trimmed) return null

  let payload: unknown
  try {
    payload = JSON.parse(trimmed)
    if (typeof payload === 'string') payload = JSON.parse(payload)
  } catch {
    return splitThinkingTags(body)
  }
  if (!isRecord(payload)) return null

  const parsers = [
    parseOpenAIResponse,
    parseAnthropicResponse,
    parseGeminiResponse,
    parseResponsesApiResponse,
  ]
  for (const parser of parsers) {
    const parts = parser(payload)
    if (parts.length > 0) return parts
  }
  return null
}
