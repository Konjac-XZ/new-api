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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, HeartPulse, Loader2, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import { formatTimestampToDate } from '@/lib/format'

import { getChannelBreakerDetail, resetChannelBreaker } from '../../api'
import { channelsQueryKeys } from '../../lib'
import type { ChannelBreakerState } from '../../types'
import { useChannels } from '../channels-provider'

type ChannelBreakerStatusDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type PhaseMeta = {
  label: string
  variant: StatusVariant
}

const PHASE_META: Record<string, PhaseMeta> = {
  disabled: { label: 'Disabled', variant: 'neutral' },
  closed: { label: 'Closed', variant: 'success' },
  cooling: { label: 'Cooling', variant: 'danger' },
  awaiting_probe: { label: 'Awaiting probe', variant: 'warning' },
  observation: { label: 'Observation', variant: 'warning' },
}

function formatPercent(value: number | undefined): string {
  if (!Number.isFinite(value)) return '-'
  return `${((value || 0) * 100).toFixed(1)}%`
}

function formatNumber(value: number | undefined, digits = 2): string {
  if (!Number.isFinite(value)) return '-'
  return (value || 0).toFixed(digits)
}

function formatSeconds(value: number | undefined): string {
  const total = Math.max(0, Math.floor(Number(value) || 0))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
  }
  return `${minutes}:${String(seconds).padStart(2, '0')}`
}

function getPhaseMeta(phase: string): PhaseMeta {
  return PHASE_META[phase] ?? { label: phase || 'Status', variant: 'neutral' }
}

function StatItem(props: {
  label: string
  value: string | number
  hint?: string
}) {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 text-sm font-semibold tabular-nums'>
        {props.value}
      </div>
      {props.hint && (
        <div className='text-muted-foreground mt-1 text-xs'>{props.hint}</div>
      )}
    </div>
  )
}

function BreakerStatePanel(props: { state: ChannelBreakerState }) {
  const { t } = useTranslation()
  const phase = props.state.phase || 'disabled'
  const phaseMeta = getPhaseMeta(phase)
  const hpMax = props.state.max_hp || 10
  const hp = props.state.hp ?? hpMax
  const hpPercent =
    hpMax > 0 ? Math.max(0, Math.min(100, (hp / hpMax) * 100)) : 100
  const cooldownPercent =
    props.state.cooldown_seconds > 0
      ? Math.max(
          0,
          Math.min(
            100,
            (props.state.remaining_cooldown_seconds /
              props.state.cooldown_seconds) *
              100
          )
        )
      : 0

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <div className='flex items-center gap-2'>
            <HeartPulse className='text-info h-4 w-4' />
            <span className='text-sm font-semibold'>
              {t('Dynamic circuit breaker enabled')}
            </span>
          </div>
          <div className='text-muted-foreground mt-1 text-xs'>
            {t('Updated')}: {formatTimestampToDate(props.state.updated_at)}
          </div>
        </div>
        <StatusBadge
          label={t(phaseMeta.label)}
          variant={phaseMeta.variant}
          copyable={false}
        />
      </div>

      {phase === 'cooling' && (
        <div className='space-y-1.5'>
          <div className='flex items-center justify-between text-xs'>
            <span>{t('Cooldown')}</span>
            <span className='text-muted-foreground tabular-nums'>
              {t('Remaining')}:{' '}
              {formatSeconds(props.state.remaining_cooldown_seconds)}
            </span>
          </div>
          <Progress value={cooldownPercent} />
        </div>
      )}

      <div className='space-y-1.5'>
        <div className='flex items-center justify-between text-xs'>
          <span>HP</span>
          <span className='text-muted-foreground tabular-nums'>
            {formatNumber(hp, 1)} / {formatNumber(hpMax, 1)}
          </span>
        </div>
        <Progress value={hpPercent} />
      </div>

      <div className='grid gap-2 sm:grid-cols-2'>
        <StatItem label={t('Status')} value={t(phaseMeta.label)} />
        <StatItem
          label={t('Pressure')}
          value={formatNumber(props.state.pressure)}
          hint={`${t('Failed')}: ${props.state.fail_streak || 0}`}
        />
        <StatItem
          label={t('Failure rate')}
          value={formatPercent(props.state.failure_rate)}
        />
        <StatItem
          label={t('Timeout rate')}
          value={formatPercent(props.state.timeout_rate)}
        />
        <StatItem
          label={t('Weight')}
          value={props.state.effective_weight || 0}
          hint={`${t('Base weight')}: ${props.state.base_weight || 0}`}
        />
        <StatItem
          label={t('Health')}
          value={formatPercent(props.state.hp_ratio)}
          hint={`${t('Tolerance')}: ${formatNumber(
            props.state.tolerance_coefficient,
            1
          )}`}
        />
      </div>

      <Separator />

      <div className='grid gap-2 sm:grid-cols-2'>
        <StatItem label={t('Type')} value={props.state.last_failure || '-'} />
        <StatItem
          label={t('Reset')}
          value={props.state.trip_count || 0}
          hint={formatTimestampToDate(props.state.cooldown_at)}
        />
      </div>
    </div>
  )
}

export function ChannelBreakerStatusDialog(
  props: ChannelBreakerStatusDialogProps
) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const queryClient = useQueryClient()
  const [resetConfirmOpen, setResetConfirmOpen] = useState(false)
  const [resetLoading, setResetLoading] = useState(false)
  const channelId = currentRow?.id

  const detailQuery = useQuery({
    queryKey: channelId
      ? [...channelsQueryKeys.detail(channelId), 'breaker']
      : ['channels', 'detail', 'breaker', 'none'],
    queryFn: async () => {
      if (!channelId) throw new Error('missing channel id')
      const response = await getChannelBreakerDetail(channelId)
      if (!response.success) {
        throw new Error(response.message || t('Failed to load'))
      }
      return response.data
    },
    enabled: props.open && Boolean(channelId),
  })

  const breakerState = useMemo(
    () => detailQuery.data?.breaker_state ?? currentRow?.breaker_state ?? null,
    [detailQuery.data?.breaker_state, currentRow?.breaker_state]
  )

  const handleReset = async () => {
    if (!channelId) return
    setResetLoading(true)
    try {
      const response = await resetChannelBreaker(channelId)
      if (response.success) {
        toast.success(t('Reset completed'))
        setResetConfirmOpen(false)
        await Promise.all([
          detailQuery.refetch(),
          queryClient.invalidateQueries({
            queryKey: channelsQueryKeys.lists(),
          }),
        ])
      } else {
        toast.error(response.message || t('Reset failed'))
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Reset failed'))
    } finally {
      setResetLoading(false)
    }
  }

  let content = (
    <div className='text-muted-foreground py-8 text-sm'>{t('No data')}</div>
  )
  if (detailQuery.isLoading) {
    content = (
      <div className='text-muted-foreground flex items-center gap-2 py-8 text-sm'>
        <Loader2 className='h-4 w-4 animate-spin' />
        {t('Loading')}
      </div>
    )
  } else if (breakerState) {
    content = <BreakerStatePanel state={breakerState} />
  }

  return (
    <>
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent className='sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>
              <div className='flex items-center gap-2'>
                <Activity className='h-4 w-4' />
                <span>{t('Dynamic Breaker')}</span>
              </div>
            </DialogTitle>
            {currentRow?.name && (
              <div className='text-muted-foreground text-sm'>
                {currentRow.name}
              </div>
            )}
          </DialogHeader>

          {content}

          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => detailQuery.refetch()}
              disabled={detailQuery.isFetching}
            >
              {detailQuery.isFetching ? (
                <Loader2 className='h-4 w-4 animate-spin' />
              ) : (
                <RefreshCw className='h-4 w-4' />
              )}
              {t('Refresh')}
            </Button>
            <Button
              type='button'
              variant='destructive'
              onClick={() => setResetConfirmOpen(true)}
              disabled={!breakerState?.dynamic_enabled || resetLoading}
            >
              {t('Reset')}
            </Button>
            <DialogClose render={<Button type='button' variant='outline' />}>
              {t('Close')}
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={resetConfirmOpen}
        onOpenChange={setResetConfirmOpen}
        title={t('Reset')}
        desc={t('Reset dynamic breaker runtime state for this channel?')}
        confirmText={t('Reset')}
        destructive
        isLoading={resetLoading}
        handleConfirm={handleReset}
      />
    </>
  )
}
