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
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'

import {
  CHANNEL_SORT_FIELD_LABELS,
  CHANNEL_SORT_FIELDS,
  DEFAULT_CHANNEL_SORT_RULES,
  EXAMPLE_CHANNEL_SORT_RULES,
  normalizeChannelSortRules,
} from '../lib'
import type { ChannelSortBy, ChannelSortOrder, ChannelSortRule } from '../types'

type ChannelSortDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  rules: ChannelSortRule[]
  onApply: (rules: ChannelSortRule[]) => void
}

function getNextField(rules: ChannelSortRule[]): ChannelSortBy | undefined {
  const usedFields = new Set(rules.map((rule) => rule.field))
  return CHANNEL_SORT_FIELDS.find((field) => !usedFields.has(field))
}

export function ChannelSortDialog(props: ChannelSortDialogProps) {
  const { t } = useTranslation()
  const [draftRules, setDraftRules] = useState<ChannelSortRule[]>(() =>
    normalizeChannelSortRules(props.rules)
  )

  useEffect(() => {
    if (props.open) {
      setDraftRules(normalizeChannelSortRules(props.rules))
    }
  }, [props.open, props.rules])

  const canAddRule = useMemo(
    () => getNextField(draftRules) !== undefined,
    [draftRules]
  )

  const updateRule = (index: number, patch: Partial<ChannelSortRule>): void => {
    setDraftRules((rules) =>
      normalizeChannelSortRules(
        rules.map((rule, ruleIndex) =>
          ruleIndex === index ? { ...rule, ...patch } : rule
        )
      )
    )
  }

  const moveRule = (index: number, offset: number): void => {
    setDraftRules((rules) => {
      const nextIndex = index + offset
      if (nextIndex < 0 || nextIndex >= rules.length) return rules

      const nextRules = [...rules]
      const currentRule = nextRules[index]
      nextRules[index] = nextRules[nextIndex]
      nextRules[nextIndex] = currentRule
      return nextRules
    })
  }

  const addRule = (): void => {
    setDraftRules((rules) => {
      const nextField = getNextField(rules)
      if (!nextField) return rules
      return [...rules, { field: nextField, order: 'desc' }]
    })
  }

  const deleteRule = (index: number): void => {
    setDraftRules((rules) =>
      normalizeChannelSortRules(
        rules.filter((_rule, ruleIndex) => ruleIndex !== index)
      )
    )
  }

  const applyRules = (): void => {
    props.onApply(draftRules)
    props.onOpenChange(false)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {t('Sort')} {t('Rules')}
          </DialogTitle>
        </DialogHeader>

        <div className='space-y-2'>
          {draftRules.map((rule, index) => {
            const selectedFields = new Set(
              draftRules
                .filter((_item, ruleIndex) => ruleIndex !== index)
                .map((item) => item.field)
            )

            return (
              <div
                key={rule.field}
                className='grid gap-2 rounded-lg border p-3 sm:grid-cols-[auto_minmax(0,1fr)_8rem_auto] sm:items-center'
              >
                <div className='text-muted-foreground w-16 text-sm font-medium'>
                  {t('Rule')} {index + 1}
                </div>

                <NativeSelect
                  value={rule.field}
                  onChange={(event) =>
                    updateRule(index, {
                      field: event.target.value as ChannelSortBy,
                    })
                  }
                  aria-label={t('Sort')}
                  className='w-full'
                >
                  {CHANNEL_SORT_FIELDS.map((field) => (
                    <NativeSelectOption
                      key={field}
                      value={field}
                      disabled={selectedFields.has(field)}
                    >
                      {t(CHANNEL_SORT_FIELD_LABELS[field])}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>

                <NativeSelect
                  value={rule.order}
                  onChange={(event) =>
                    updateRule(index, {
                      order: event.target.value as ChannelSortOrder,
                    })
                  }
                  aria-label={t('Sort Order')}
                  className='w-full'
                >
                  <NativeSelectOption value='desc'>
                    {t('Desc')}
                  </NativeSelectOption>
                  <NativeSelectOption value='asc'>
                    {t('Asc')}
                  </NativeSelectOption>
                </NativeSelect>

                <div className='flex items-center gap-1 sm:justify-end'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={index === 0}
                    onClick={() => moveRule(index, -1)}
                    aria-label={t('Move')}
                  >
                    <ArrowUp className='h-4 w-4' />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={index === draftRules.length - 1}
                    onClick={() => moveRule(index, 1)}
                    aria-label={t('Move')}
                  >
                    <ArrowDown className='h-4 w-4' />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={draftRules.length === 1}
                    onClick={() => deleteRule(index)}
                    aria-label={t('Delete')}
                  >
                    <Trash2 className='h-4 w-4' />
                  </Button>
                </div>
              </div>
            )
          })}
        </div>

        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={addRule}
            disabled={!canAddRule}
          >
            <Plus className='h-4 w-4' />
            {t('Add Rule')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => setDraftRules([...DEFAULT_CHANNEL_SORT_RULES])}
          >
            {t('Reset to default')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => setDraftRules([...EXAMPLE_CHANNEL_SORT_RULES])}
          >
            {t('Example')}
          </Button>
        </div>

        <DialogFooter>
          <DialogClose render={<Button type='button' variant='outline' />}>
            {t('Cancel')}
          </DialogClose>
          <Button type='button' onClick={applyRules}>
            {t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
