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

import { getMonitorTableLayout } from '../table-layout'

type TestRow = { model: string }

const compactColumns = [
  { key: 'status', label: 'Status', layout: { min: 7, max: 10 } },
  {
    key: 'model',
    label: 'Model',
    layout: { min: 16, max: 32, contentScale: 0.72 },
    measure: (row: TestRow) => row.model,
  },
  { key: 'channel', label: 'Channel', layout: { min: 13, max: 24 } },
  { key: 'tokens', label: 'Input / Output', layout: { min: 13, max: 15 } },
  { key: 'duration', label: 'Duration', layout: { min: 9, max: 9 } },
] as const

describe('Monitor table layout', () => {
  test('sizes a compact five-column selection from only its visible columns', () => {
    const layout = getMonitorTableLayout(compactColumns, [
      { model: 'gemini-3.5-flash-lite' },
    ])

    assert.equal(layout.minWidthRem, 47.2)
    assert.equal(
      Object.values(layout.widthPercents).reduce(
        (total, width) => total + width,
        0
      ),
      100
    )
  })

  test('allows long visible content to grow the scrollable minimum width', () => {
    const layout = getMonitorTableLayout(compactColumns, [
      { model: 'a'.repeat(100) },
    ])

    assert.equal(layout.minWidthRem, 60)
    assert.equal(layout.widthPercents.model > layout.widthPercents.status, true)
  })
})
