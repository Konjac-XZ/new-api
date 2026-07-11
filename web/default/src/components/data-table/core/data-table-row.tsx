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
import {
  flexRender,
  type Row,
  type Table as TanstackTable,
} from '@tanstack/react-table'
import * as React from 'react'

import { TableCell, TableRow } from '@/components/design-system/table'
import { cn } from '@/lib/utils'

import type { DataTableColumnClassName } from './types'

type DataTableRowProps<TData> = {
  row: Row<TData>
  className?: string
  getColumnClassName?: DataTableColumnClassName
  cellRenderColumns?: TanstackTable<TData>['options']['columns']
  visibleColumnIds?: string
} & Omit<React.ComponentProps<typeof TableRow>, 'children'>

type DataTableRowInnerProps<TData> = DataTableRowProps<TData> & {
  isSelected: boolean
}

function DataTableRowInner<TData>({
  row,
  isSelected,
  className,
  getColumnClassName,
  cellRenderColumns,
  visibleColumnIds,
  ...rowProps
}: DataTableRowInnerProps<TData>) {
  // Destructured only to keep it out of `rowProps` (it is not a valid DOM attr)
  // and to feed the memo comparator below; it is intentionally unused here.
  void cellRenderColumns
  void visibleColumnIds

  const visibleCells = row
    .getAllCells()
    .filter((cell) => cell.column.getIsVisible())

  return (
    <TableRow
      data-state={isSelected ? 'selected' : undefined}
      className={className}
      {...rowProps}
    >
      {visibleCells.map((cell) => {
        const contentMode = cell.column.columnDef.meta?.contentMode ?? 'wrap'

        return (
          <TableCell
            key={cell.id}
            data-column-id={cell.column.id}
            data-content-mode={contentMode}
            className={cn(
              'max-w-full min-w-0',
              contentMode === 'full' &&
                'max-w-none overflow-visible [&_.truncate]:overflow-visible [&_.truncate]:text-clip',
              contentMode === 'wrap' &&
                'whitespace-normal break-words [overflow-wrap:anywhere] [&_.truncate]:overflow-visible [&_.truncate]:text-clip [&_.truncate]:whitespace-normal',
              contentMode === 'summary' &&
                'whitespace-normal break-words [overflow-wrap:anywhere]',
              getColumnClassName?.(cell.column.id, 'cell')
            )}
          >
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </TableCell>
        )
      })}
    </TableRow>
  )
}

const MemoizedDataTableRow = React.memo(DataTableRowInner, (prev, next) => {
  // Do not read row.getIsSelected() inside the comparator: TanStack row objects
  // keep a stable reference while their selection state mutates, so reading it
  // here compares identical live values and misses selection changes. Selection
  // is lifted to the `isSelected` prop, captured per render in DataTableRow.
  //
  // Column cell renderers (and getColumnClassName) can close over external
  // state while the row stays stable, so column definitions and the class
  // resolver are part of the render identity and must be compared too.
  return (
    prev.row === next.row &&
    prev.className === next.className &&
    prev.isSelected === next.isSelected &&
    prev.getColumnClassName === next.getColumnClassName &&
    prev.cellRenderColumns === next.cellRenderColumns &&
    prev.visibleColumnIds === next.visibleColumnIds
  )
}) as typeof DataTableRowInner

export function DataTableRow<TData>(props: DataTableRowProps<TData>) {
  return (
    <MemoizedDataTableRow {...props} isSelected={props.row.getIsSelected()} />
  )
}
