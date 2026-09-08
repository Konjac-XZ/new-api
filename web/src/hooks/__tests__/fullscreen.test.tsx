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
  'Node',
  'Element',
  'Event',
  'KeyboardEvent',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act, useRef } = await import('react')
const { createRoot } = await import('react-dom/client')
const { useFullscreen } = await import('../use-fullscreen')
const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type RenderedHarness = {
  button: HTMLButtonElement
  container: HTMLDivElement
  root: ReturnType<typeof createRoot>
  target: HTMLDivElement
}

function FullscreenHarness() {
  const targetRef = useRef<HTMLDivElement | null>(null)
  const fullscreen = useFullscreen(targetRef)

  return (
    <div ref={targetRef} data-active={fullscreen.isFullscreen}>
      <button type='button' onClick={fullscreen.toggleFullscreen}>
        Toggle fullscreen
      </button>
    </div>
  )
}

async function renderHarness(): Promise<RenderedHarness> {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => root.render(<FullscreenHarness />))

  const target = container.querySelector('div')
  const button = container.querySelector('button')
  assert.ok(target)
  assert.ok(button)
  return { button, container, root, target }
}

async function unmountHarness(rendered: RenderedHarness) {
  await act(async () => rendered.root.unmount())
  rendered.container.remove()
}

describe('Shared fullscreen behavior', () => {
  after(() => {
    domWindow.close()
  })

  test('uses an in-page focus view on desktop and exits it with Escape', async () => {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({ matches: true }),
    })
    let nativeRequestCount = 0
    const rendered = await renderHarness()
    rendered.target.requestFullscreen = async () => {
      nativeRequestCount += 1
    }

    await act(async () => rendered.button.click())

    assert.equal(rendered.target.dataset.active, 'true')
    assert.equal(nativeRequestCount, 0)

    await act(async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    })
    assert.equal(rendered.target.dataset.active, 'false')

    await unmountHarness(rendered)
  })

  test('keeps native fullscreen for touch-first devices', async () => {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({ matches: false }),
    })
    let fullscreenElement: Element | null = null
    Object.defineProperty(document, 'fullscreenElement', {
      configurable: true,
      get: () => fullscreenElement,
    })
    const rendered = await renderHarness()
    rendered.target.requestFullscreen = async () => {
      fullscreenElement = rendered.target
      document.dispatchEvent(new Event('fullscreenchange'))
    }

    await act(async () => rendered.button.click())

    assert.equal(fullscreenElement, rendered.target)
    assert.equal(rendered.target.dataset.active, 'true')

    await unmountHarness(rendered)
    fullscreenElement = null
  })
})
