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
import { ChevronDown } from 'lucide-react'
import type { ReactElement } from 'react'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Label } from '@/components/ui/label'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export type ModelReconciliationRow = {
  id: string
  model: string
  checked: boolean
  className: string
  badgeLabel?: string
  badgeClassName?: string
  hint?: ReactElement
  hintText?: string
}

type ModelReconciliationListProps = {
  categories: Array<[string, ModelReconciliationRow[]]>
  selectedLabel: string
  onToggleRow: (row: ModelReconciliationRow) => void
  onToggleCategory: (rows: ModelReconciliationRow[], checked: boolean) => void
}

export function ModelReconciliationList(props: ModelReconciliationListProps) {
  return props.categories.map(([categoryName, rows]) => {
    const allSelected = rows.length > 0 && rows.every((row) => row.checked)
    const selectedCount = rows.filter((row) => row.checked).length

    return (
      <Collapsible key={categoryName} defaultOpen>
        <CollapsibleTrigger className='hover:bg-muted/50 flex w-full items-center justify-between gap-3 rounded-lg border p-3'>
          <div className='flex min-w-0 items-center gap-2'>
            <ChevronDown className='h-4 w-4 shrink-0' />
            <span className='min-w-0 truncate font-medium'>
              {categoryName} ({rows.length})
            </span>
          </div>
          <div className='flex shrink-0 items-center gap-2'>
            <span className='text-muted-foreground text-sm whitespace-nowrap'>
              {selectedCount} / {rows.length} {props.selectedLabel}
            </span>
            <Checkbox
              checked={allSelected}
              onCheckedChange={(checked) =>
                props.onToggleCategory(rows, Boolean(checked))
              }
              onClick={(event) => event.stopPropagation()}
            />
          </div>
        </CollapsibleTrigger>
        <CollapsibleContent className='px-1 py-2 sm:px-4'>
          <div className='grid grid-cols-[repeat(auto-fit,minmax(min(100%,18rem),1fr))] gap-2'>
            {rows.map((row) => (
              <div key={row.id} className='flex min-w-0 items-start gap-2'>
                <Checkbox
                  id={row.id}
                  checked={row.checked}
                  onCheckedChange={() => props.onToggleRow(row)}
                  className='mt-0.5 shrink-0'
                />
                <Label
                  htmlFor={row.id}
                  className='flex min-w-0 flex-1 cursor-pointer flex-col items-start gap-1 text-sm leading-5 font-normal'
                >
                  <span
                    className={cn(
                      'min-w-0 break-words [overflow-wrap:anywhere]',
                      row.className
                    )}
                  >
                    {row.model}
                  </span>
                  {row.hint ? (
                    <Tooltip>
                      <TooltipTrigger render={row.hint} />
                      <TooltipContent>{row.hintText}</TooltipContent>
                    </Tooltip>
                  ) : null}
                  {row.badgeLabel ? (
                    <Badge
                      variant='outline'
                      className={cn('max-w-full', row.badgeClassName)}
                      title={row.badgeLabel}
                    >
                      <span className='min-w-0 truncate'>{row.badgeLabel}</span>
                    </Badge>
                  ) : null}
                </Label>
              </div>
            ))}
          </div>
        </CollapsibleContent>
      </Collapsible>
    )
  })
}
