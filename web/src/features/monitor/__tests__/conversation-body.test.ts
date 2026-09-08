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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { parseConversationBody } from '../conversation-body'

describe('monitor conversation body parser', () => {
  test('decodes OpenAI message escapes into display-ready Markdown text', () => {
    const body = JSON.stringify({
      model: 'gpt-5',
      messages: [
        { role: 'system', content: 'Follow **these** rules.' },
        {
          role: 'user',
          content: 'First line\nSecond line with "quoted text" and `code`',
        },
      ],
    })

    const conversation = parseConversationBody(body)

    assert.ok(conversation)
    assert.equal(conversation.messages.length, 2)
    assert.equal(conversation.messages[0].role, 'system')
    assert.deepEqual(conversation.messages[1].parts, [
      {
        kind: 'text',
        text: 'First line\nSecond line with "quoted text" and `code`',
      },
    ])
  })

  test('normalizes Anthropic system blocks, thinking, tools, and tool results', () => {
    const body = JSON.stringify({
      system: [{ type: 'text', text: 'Be concise.' }],
      messages: [
        {
          role: 'assistant',
          content: [
            { type: 'thinking', thinking: 'Check the **inputs**.' },
            {
              type: 'tool_use',
              id: 'tool-1',
              name: 'lookup',
              input: { query: 'weather' },
            },
          ],
        },
        {
          role: 'user',
          content: [
            {
              type: 'tool_result',
              tool_use_id: 'tool-1',
              content: [{ type: 'text', text: 'Sunny\n24°C' }],
            },
          ],
        },
      ],
    })

    const conversation = parseConversationBody(body)

    assert.ok(conversation)
    assert.equal(conversation.messages.length, 3)
    assert.deepEqual(conversation.messages[0], {
      originalRole: 'system',
      parts: [{ kind: 'text', text: 'Be concise.' }],
      role: 'system',
    })
    assert.deepEqual(conversation.messages[1].parts, [
      { kind: 'thinking', text: 'Check the **inputs**.' },
      { kind: 'json', label: 'lookup', text: '{\n  "query": "weather"\n}' },
    ])
    assert.deepEqual(conversation.messages[2].parts, [
      { kind: 'text', label: 'tool-1', text: 'Sunny\n24°C' },
    ])
  })

  test('falls back when the body is not a supported message request', () => {
    assert.equal(parseConversationBody('{"prompt":"hello"}'), null)
    assert.equal(parseConversationBody('not json'), null)
    assert.equal(parseConversationBody('{"messages":[]}'), null)
  })
})
