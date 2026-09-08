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

import { parseResponseBody } from '../response-body'

describe('monitor response body parser', () => {
  test('separates tagged thinking code blocks from visible content', () => {
    const body =
      '<thinking>\n```ts\nconst answer = 42\n```\n</thinking>\nThe answer is **42**.'

    assert.deepEqual(parseResponseBody(body), [
      {
        kind: 'thinking',
        text: '\n```ts\nconst answer = 42\n```\n',
      },
      { kind: 'content', text: '\nThe answer is **42**.' },
    ])
  })

  test('extracts OpenAI reasoning fields without losing the final answer', () => {
    const body = JSON.stringify({
      choices: [
        {
          message: {
            role: 'assistant',
            reasoning_content: 'Check the inputs.',
            content: 'Done.',
          },
        },
      ],
    })

    assert.deepEqual(parseResponseBody(body), [
      { kind: 'thinking', text: 'Check the inputs.' },
      { kind: 'content', text: 'Done.' },
    ])
  })

  test('extracts Anthropic and Gemini native thinking blocks', () => {
    const anthropic = JSON.stringify({
      content: [
        { type: 'thinking', thinking: 'Reason first.' },
        { type: 'text', text: 'Answer second.' },
      ],
    })
    const gemini = JSON.stringify({
      candidates: [
        {
          content: {
            parts: [
              { thought: true, text: 'Reason first.' },
              { text: 'Answer second.' },
            ],
          },
        },
      ],
    })

    const expected = [
      { kind: 'thinking', text: 'Reason first.' },
      { kind: 'content', text: 'Answer second.' },
    ]
    assert.deepEqual(parseResponseBody(anthropic), expected)
    assert.deepEqual(parseResponseBody(gemini), expected)
  })

  test('extracts Responses API reasoning summaries and output text', () => {
    const body = JSON.stringify({
      output: [
        {
          type: 'reasoning',
          summary: [{ type: 'summary_text', text: 'Reason first.' }],
        },
        {
          type: 'message',
          content: [{ type: 'output_text', text: 'Answer second.' }],
        },
      ],
    })

    assert.deepEqual(parseResponseBody(body), [
      { kind: 'thinking', text: 'Reason first.' },
      { kind: 'content', text: 'Answer second.' },
    ])
  })
})
