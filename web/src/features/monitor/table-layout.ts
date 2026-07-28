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
const COLUMN_SCORE_WIDTH_REM = 0.8

type MonitorTableLayoutColumn<Key extends string, Row> = {
  key: Key
  label: string
  layout: {
    min: number
    max: number
    contentScale?: number
  }
  measure?: (row: Row) => string
}

export function getMonitorTableLayout<Key extends string, Row>(
  columns: readonly MonitorTableLayoutColumn<Key, Row>[],
  rows: readonly Row[]
): { minWidthRem: number; widthPercents: Record<Key, number> } {
  const columnScores = columns.map((column) => {
    let measuredChars = column.label.length
    if (column.measure) {
      for (const row of rows) {
        measuredChars = Math.max(measuredChars, column.measure(row).length)
      }
    }
    const contentScore = Math.ceil(
      measuredChars * (column.layout.contentScale ?? 1)
    )
    const score = Math.min(
      column.layout.max,
      Math.max(column.layout.min, contentScore)
    )

    return { key: column.key, score }
  })
  const totalScore = columnScores.reduce((sum, column) => sum + column.score, 0)
  const minWidthRem = Math.round(totalScore * COLUMN_SCORE_WIDTH_REM * 10) / 10
  const widthPercents = Object.fromEntries(
    columnScores.map((column) => [
      column.key,
      totalScore > 0 ? (column.score / totalScore) * 100 : 0,
    ])
  ) as Record<Key, number>

  return { minWidthRem, widthPercents }
}
