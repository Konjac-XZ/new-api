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
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'HTMLTextAreaElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const i18next = (await import('i18next')).default
const { initReactI18next } = await import('react-i18next')
await i18next.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Assistant: 'Assistant',
        Conversation: 'Conversation',
        Developer: 'Developer',
        Document: 'Document',
        'Empty content': 'Empty content',
        Image: 'Image',
        Message: 'Message',
        'Raw JSON': 'Raw JSON',
        System: 'System',
        Thinking: 'Thinking',
        Tool: 'Tool',
        User: 'User',
      },
    },
  },
})
const { MessageBodyViewer } = await import('../message-body-viewer')
const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

async function renderViewer(body: string) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(<MessageBodyViewer body={body} prettyBody={body} />)
  })

  return { container, root }
}

describe('monitor message body viewer', () => {
  after(() => {
    domWindow.close()
  })

  test('shows decoded Markdown messages as an accessible conversation by default', async () => {
    const body = JSON.stringify({
      messages: [
        {
          role: 'system',
          content:
            '<SETTINGS priority="high">\nUse **Markdown** and do not emit `<thinking>`.\n</SETTINGS>',
        },
        { role: 'user', content: 'Line one\nLine two with "quotes".' },
      ],
    })
    const rendered = await renderViewer(body)

    const systemMessage = rendered.container.querySelector(
      'article[aria-label="System Message 1"]'
    )
    const userMessage = rendered.container.querySelector(
      'article[aria-label="User Message 2"]'
    )
    const viewSelector = rendered.container.querySelector('[role="tablist"]')
    assert.ok(viewSelector)
    assert.equal(viewSelector.classList.contains('ml-auto'), true)
    assert.equal(viewSelector.classList.contains('sm:absolute'), true)
    assert.equal(viewSelector.classList.contains('sm:right-0'), true)
    assert.ok(systemMessage)
    assert.ok(systemMessage.querySelector('strong'))
    assert.equal(systemMessage.querySelector('strong')?.textContent, 'Markdown')
    assert.match(systemMessage.textContent ?? '', /<SETTINGS priority="high">/)
    assert.match(systemMessage.textContent ?? '', /<thinking>/)
    assert.match(systemMessage.textContent ?? '', /<\/SETTINGS>/)
    assert.match(userMessage?.textContent ?? '', /Line one/)
    assert.match(userMessage?.textContent ?? '', /Line two with "quotes"\./)
    assert.doesNotMatch(userMessage?.textContent ?? '', /\\n/)

    const rawButton = [...rendered.container.querySelectorAll('button')].find(
      (button) => button.textContent?.includes('Raw JSON')
    )
    assert.ok(rawButton)
    await act(async () => rawButton.click())
    assert.equal(rendered.container.querySelector('textarea')?.value, body)

    await act(async () => rendered.root.unmount())
    rendered.container.remove()
  })

  test('keeps unsupported JSON in the raw read-only viewer', async () => {
    const body = '{"prompt":"hello"}'
    const rendered = await renderViewer(body)
    const textarea = rendered.container.querySelector('textarea')

    assert.ok(textarea)
    assert.equal(textarea.readOnly, true)
    assert.equal(textarea.value, body)
    assert.equal(rendered.container.querySelector('article'), null)

    await act(async () => rendered.root.unmount())
    rendered.container.remove()
  })
})
