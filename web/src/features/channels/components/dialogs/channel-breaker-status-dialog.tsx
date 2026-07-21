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
import {
  Activity,
  Gauge,
  HeartPulse,
  History,
  Loader2,
  RefreshCw,
  Route,
  ShieldAlert,
  TimerReset,
} from 'lucide-react'
import { type ComponentType, type ReactNode, useMemo, useState } from 'react'
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
import { cn } from '@/lib/utils'

import { getChannelBreakerDetail, resetChannelBreaker } from '../../api'
import { channelsQueryKeys } from '../../lib'
import type {
  BreakerPenaltyTraceDetail,
  ChannelBreakerState,
} from '../../types'
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

const FAILURE_LABELS: Record<string, string> = {
  generic: 'Generic failure',
  immediate_failure: 'Immediate failure',
  first_token_timeout: 'First token timeout',
  mid_stream_failure: 'Mid-stream failure',
  overloaded: 'Upstream overloaded',
  empty_reply: 'Empty reply',
}

const EVENT_LABELS: Record<string, string> = {
  relay_failure: 'Relay failure',
  probe_failure: 'Probe failure',
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

function formatUnixTime(value: number | undefined): string {
  if (!value || value <= 0) return '-'
  return formatTimestampToDate(value)
}

function formatLabel(
  value: string | undefined,
  labels: Record<string, string>
) {
  if (!value) return '-'
  return labels[value] || value
}

function getPhaseMeta(phase: string): PhaseMeta {
  return PHASE_META[phase] ?? { label: phase || 'Status', variant: 'neutral' }
}

function SectionTitle(props: {
  icon: ComponentType<{ className?: string }>
  title: string
  action?: ReactNode
}) {
  const Icon = props.icon
  return (
    <div className='flex items-center justify-between gap-3'>
      <div className='flex items-center gap-2'>
        <Icon className='text-muted-foreground h-4 w-4' />
        <div className='text-sm font-semibold'>{props.title}</div>
      </div>
      {props.action}
    </div>
  )
}

function MetricItem(props: {
  label: string
  value: string | number
  hint?: string
  tone?: 'default' | 'danger' | 'warning' | 'success'
}) {
  return (
    <div
      className={cn(
        'bg-background rounded-lg border p-3',
        props.tone === 'danger' && 'border-destructive/35',
        props.tone === 'warning' && 'border-warning/35',
        props.tone === 'success' && 'border-success/35'
      )}
    >
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

function RecentPenaltyList(props: {
  traces: BreakerPenaltyTraceDetail[]
  total: number
}) {
  const { t } = useTranslation()

  return (
    <div className='space-y-3'>
      <SectionTitle
        icon={History}
        title={t('Recent penalties')}
        action={
          props.total > 0 ? (
            <span className='text-muted-foreground text-xs'>
              {props.traces.length}/{props.total}
            </span>
          ) : null
        }
      />

      {props.traces.length === 0 ? (
        <div className='text-muted-foreground rounded-lg border border-dashed px-3 py-6 text-center text-sm'>
          {t('No penalty records')}
        </div>
      ) : (
        <div className='space-y-2'>
          {props.traces.slice(0, 3).map((trace) => {
            const hpBefore = formatNumber(trace.hp_before, 1)
            const hpAfter = formatNumber(trace.hp_after, 1)
            const pressureBefore = formatNumber(trace.pressure_before, 2)
            const pressureAfter = formatNumber(trace.pressure_after, 2)
            return (
              <div key={trace.id} className='rounded-lg border p-3'>
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <div className='flex min-w-0 items-center gap-2'>
                    <StatusBadge
                      label={t(formatLabel(trace.event_type, EVENT_LABELS))}
                      variant='info'
                      size='sm'
                      copyable={false}
                    />
                    <StatusBadge
                      label={
                        trace.triggered_cooldown
                          ? t('Triggered cooldown')
                          : t('No cooldown')
                      }
                      variant={trace.triggered_cooldown ? 'danger' : 'neutral'}
                      size='sm'
                      copyable={false}
                    />
                  </div>
                  <span className='text-muted-foreground text-xs'>
                    {formatUnixTime(trace.created_at)}
                  </span>
                </div>
                <div className='mt-3 grid gap-x-4 gap-y-2 text-xs sm:grid-cols-2'>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Last failure')}:
                    </span>{' '}
                    <span>
                      {t(formatLabel(trace.failure_kind, FAILURE_LABELS))}
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Final cooldown')}:
                    </span>{' '}
                    <span>{formatSeconds(trace.final_cooldown_seconds)}</span>
                  </div>
                  <div>
                    <span className='text-muted-foreground'>HP:</span>{' '}
                    <span>
                      {hpBefore} {'->'} {hpAfter}
                    </span>
                    {trace.hp_damage > 0 && (
                      <span className='text-destructive ml-1'>
                        -{formatNumber(trace.hp_damage, 1)}
                      </span>
                    )}
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Pressure')}:
                    </span>{' '}
                    <span>
                      {pressureBefore} {'->'} {pressureAfter}
                    </span>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function BreakerStatePanel(props: {
  state: ChannelBreakerState
  traces: BreakerPenaltyTraceDetail[]
  traceTotal: number
}) {
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
  const failureKind = formatLabel(props.state.last_failure, FAILURE_LABELS)
  const isDynamicEnabled = props.state.dynamic_enabled
  const effectiveWeight = props.state.effective_weight || 0
  const baseWeight = props.state.base_weight || 0

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-start justify-between gap-3 rounded-lg border p-3'>
        <div className='min-w-0'>
          <div className='flex items-center gap-2 text-sm font-semibold'>
            <HeartPulse className='text-info h-4 w-4' />
            {isDynamicEnabled
              ? t('Dynamic circuit breaker enabled')
              : t('Disabled')}
          </div>
          <div className='text-muted-foreground mt-1 text-xs'>
            {t('Updated')}: {formatUnixTime(props.state.updated_at)}
          </div>
          {phase === 'observation' && (
            <div className='text-muted-foreground mt-1 text-xs'>
              {t('Observation elapsed')}:{' '}
              {formatSeconds(props.state.observation_elapsed_seconds)}
            </div>
          )}
        </div>
        <StatusBadge
          label={t(phaseMeta.label)}
          variant={phaseMeta.variant}
          copyable={false}
        />
      </div>

      <div className='space-y-3'>
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
            <div className='text-muted-foreground text-xs'>
              {t('Cooldown ends')}: {formatUnixTime(props.state.cooldown_at)}
            </div>
          </div>
        )}

        <div className='space-y-1.5'>
          <div className='flex items-center justify-between text-xs'>
            <span>{t('Current HP')}</span>
            <span className='text-muted-foreground tabular-nums'>
              {formatNumber(hp, 1)} / {formatNumber(hpMax, 1)}
            </span>
          </div>
          <Progress value={hpPercent} />
        </div>
      </div>

      <div className='space-y-2'>
        <SectionTitle icon={Gauge} title={t('Recent signals')} />
        <div className='grid gap-2 sm:grid-cols-3'>
          <MetricItem
            label={t('Pressure')}
            value={formatNumber(props.state.pressure)}
            tone={props.state.pressure > 0 ? 'warning' : 'default'}
          />
          <MetricItem
            label={t('Fail streak')}
            value={props.state.fail_streak || 0}
            hint={`${t('Last failure')}: ${t(failureKind)}`}
            tone={props.state.fail_streak > 0 ? 'danger' : 'default'}
          />
          <MetricItem
            label={t('Trip count')}
            value={props.state.trip_count || 0}
            hint={`${t('Tolerance')}: ${formatNumber(
              props.state.tolerance_coefficient,
              1
            )}`}
          />
          <MetricItem
            label={t('Failure rate')}
            value={formatPercent(props.state.failure_rate)}
          />
          <MetricItem
            label={t('Timeout rate')}
            value={formatPercent(props.state.timeout_rate)}
          />
          <MetricItem
            label={t('Health')}
            value={formatPercent(props.state.hp_ratio)}
            tone={props.state.hp_ratio < 0.35 ? 'danger' : 'default'}
          />
        </div>
      </div>

      <div className='space-y-2'>
        <SectionTitle icon={Route} title={t('Routing impact')} />
        <div className='grid gap-2 sm:grid-cols-3'>
          <MetricItem
            label={t('Effective weight')}
            value={effectiveWeight}
            hint={`${t('Base weight')}: ${baseWeight}`}
          />
          <MetricItem
            label={t('Rate penalty factor')}
            value={formatNumber(props.state.rate_penalty_factor, 3)}
          />
          <MetricItem
            label={t('Confidence multiplier')}
            value={formatNumber(props.state.confidence_multiplier, 3)}
          />
        </div>
      </div>

      <Separator />

      <RecentPenaltyList traces={props.traces} total={props.traceTotal} />
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
  const traces = detailQuery.data?.trace_page?.items ?? []
  const traceTotal = detailQuery.data?.trace_page?.total ?? 0

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
    content = (
      <BreakerStatePanel
        state={breakerState}
        traces={traces}
        traceTotal={traceTotal}
      />
    )
  }

  return (
    <>
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-3xl'>
          <DialogHeader>
            <DialogTitle>
              <div className='flex items-center gap-2'>
                <Activity className='h-4 w-4' />
                <span>{t('Dynamic Breaker')}</span>
              </div>
            </DialogTitle>
            {currentRow?.name && (
              <div className='text-muted-foreground flex items-center gap-2 text-sm'>
                <ShieldAlert className='h-3.5 w-3.5 shrink-0' />
                <span className='min-w-0 truncate'>{currentRow.name}</span>
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
              <TimerReset className='h-4 w-4' />
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
