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
import { flexRender, type Row } from '@tanstack/react-table'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { CHANNEL_STATUS } from '../constants'
import { isTagAggregateRow } from '../lib'
import type { Channel } from '../types'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'

/**
 * Bespoke channel card for the card view. Reuses every column's existing cell
 * renderer via `flexRender`, so the table's information and interactions are
 * preserved: row selection, provider/multi-key/IO.NET type badge, id,
 * name/remark + warning icons, status (with tooltips), groups, inline
 * priority/weight spinners, balance refresh, response/test times, tag
 * expand-collapse, and the per-row (or per-tag) actions menu.
 */
function ChannelCardComponent({
  row,
  isSelected,
}: {
  row: Row<Channel>
  isSelected: boolean
}) {
  const { t } = useTranslation()
  const isTagRow = isTagAggregateRow(row.original)
  const cells = row.getVisibleCells()

  const renderCell = (id: string) => {
    const cell = cells.find((c) => c.column.id === id)
    if (!cell || !cell.column.columnDef.cell) {
      return null
    }
    return flexRender(cell.column.columnDef.cell, cell.getContext())
  }

  const fieldLabels: Record<string, string> = {
    id: t('ID'),
    models: t('Models'),
    group: t('Groups'),
    tag: t('Tag'),
    balance: t('Used / Remaining'),
    response_time: t('Response'),
    test_time: t('Last Tested'),
    priority: t('Priority'),
    weight: t('Weight'),
  }

  const selectCell = renderCell('select')
  const idCell = renderCell('id')
  const typeCell = renderCell('type')
  const nameCell = renderCell('name')
  const remarkCell = renderCell('remark')
  const statusCell = renderCell('status')
  const actionsCell = renderCell('actions')
  const modelsCell = renderCell('models')
  const groupCell = renderCell('group')
  const tagCell = renderCell('tag')
  const priorityCell = renderCell('priority')
  const weightCell = renderCell('weight')
  const balanceCell = renderCell('balance')
  const responseCell = renderCell('response_time')
  const testCell = renderCell('test_time')

  const labelClass = 'text-muted-foreground text-[11px] font-medium select-none'
  const statCells = [
    { id: 'priority', content: priorityCell },
    { id: 'weight', content: weightCell },
    { id: 'response_time', content: responseCell },
    { id: 'test_time', content: testCell },
  ].filter((item) => item.content)
  const secondaryCells = [
    { id: 'models', content: modelsCell },
    { id: 'group', content: groupCell },
    { id: 'tag', content: tagCell },
  ].filter((item) => item.content)

  // In card view the enable/disable state is already conveyed by the inline
  // power toggle, so the plain "Enabled"/"Disabled" badge is redundant. Keep
  // only the informative states (e.g. auto-disabled, unknown) and tag rows.
  const dynamicBreakerPhase = row.original.breaker_state?.phase
  const showDynamicBreakerStatus =
    row.original.status === CHANNEL_STATUS.ENABLED &&
    row.original.breaker_state?.dynamic_enabled === true &&
    (dynamicBreakerPhase === 'cooling' ||
      dynamicBreakerPhase === 'awaiting_probe' ||
      dynamicBreakerPhase === 'observation')
  const showStatusBadge =
    isTagRow ||
    showDynamicBreakerStatus ||
    (row.original.status !== CHANNEL_STATUS.ENABLED &&
      row.original.status !== CHANNEL_STATUS.MANUAL_DISABLED)

  return (
    <ChannelRowActionsLayoutContext.Provider value='card'>
      <div
        data-state={isSelected ? 'selected' : undefined}
        className='flex flex-col gap-3'
      >
        {/* Row 1: selection + type, with status badge + actions menu */}
        <div className='flex items-center justify-between gap-2'>
          <div className='flex min-w-0 flex-1 items-center gap-2'>
            {!isTagRow && selectCell && (
              <span className='shrink-0'>{selectCell}</span>
            )}
            {typeCell && (
              <div className='min-w-0 overflow-hidden'>{typeCell}</div>
            )}
          </div>
          <div className='flex shrink-0 items-center gap-1.5'>
            {showStatusBadge && statusCell}
            {actionsCell}
          </div>
        </div>

        {/* Body: left column (id/name + balance) paired with a right-aligned
          column (priority/weight + response/test time). */}
        <div className='flex items-start justify-between gap-3'>
          {/* Left column */}
          <div className='flex min-w-0 flex-1 flex-col gap-3 overflow-hidden'>
            {(idCell || nameCell || remarkCell) && (
              <div className='min-w-0 space-y-1 text-sm'>
                {idCell && (
                  <div>
                    <div className={labelClass}>{fieldLabels.id}</div>
                    {idCell}
                  </div>
                )}
                {nameCell}
                {remarkCell}
              </div>
            )}
            {balanceCell && (
              <div className='min-w-0'>
                <div className={`mb-1 ${labelClass}`}>
                  {fieldLabels.balance}
                </div>
                <div className='min-w-0 overflow-hidden text-sm'>
                  {balanceCell}
                </div>
              </div>
            )}
          </div>

          {statCells.length > 0 && (
            <div className='grid shrink-0 grid-cols-2 items-start gap-x-3 gap-y-2'>
              {statCells.map((item) => (
                <div key={item.id} className='min-w-0'>
                  <div className={labelClass}>{fieldLabels[item.id]}</div>
                  <div className='mt-1 overflow-hidden text-sm'>
                    {item.content}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {secondaryCells.length > 0 && (
          <div className='grid min-w-0 gap-2'>
            {secondaryCells.map((item) => (
              <div key={item.id} className='min-w-0'>
                <div className={labelClass}>{fieldLabels[item.id]}</div>
                <div className='mt-1 min-w-0 overflow-hidden text-sm'>
                  {item.content}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </ChannelRowActionsLayoutContext.Provider>
  )
}

/**
 * Memoized so each card only re-renders when its own react-table row reference
 * changes, instead of every card re-rendering whenever the parent table state
 * (filters, pagination, sensitive toggle, etc.) updates.
 */
export const ChannelCard = memo(ChannelCardComponent)
