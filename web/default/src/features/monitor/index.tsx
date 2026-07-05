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
  Activity,
  ArrowDownToLine,
  ArrowUpFromLine,
  Clock3,
  Expand,
  Globe2,
  Hash,
  History,
  KeyRound,
  Minimize2,
  Network,
  PauseCircle,
  Radio,
  RefreshCw,
  Route,
  Search,
  Settings2,
  Timer,
  User,
  Wifi,
  WifiOff,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn, tryPrettyJson } from '@/lib/utils'

import { getMonitorBody } from './api'
import {
  deriveDisplayStatus,
  formatBytes,
  formatDateTime,
  formatDuration,
  formatTokenCount,
  getDurationMs,
  getMonitorTokenUsage,
  getOutputSpeed,
  getRetryCount,
  getTtftMs,
  isActiveStatus,
  isTerminalStatus,
} from './lib'
import type { MonitorBodyType, MonitorRecord } from './types'
import { useMonitorWs } from './use-monitor-ws'
import { useRequestDetail } from './use-request-detail'

const RESPONSE_BODY_TABS: MonitorBodyType[] = ['response']
const MONITOR_COLUMN_STORAGE_KEY = 'monitor-table-columns'

const MONITOR_COLUMN_KEYS = {
  TIME: 'time',
  STATUS: 'status',
  MODEL: 'model',
  CHANNEL: 'channel',
  TOKEN_USAGE: 'token_usage',
  DURATION: 'duration',
  TTFT: 'ttft',
  THROUGHPUT: 'throughput',
} as const

type MonitorColumnId =
  (typeof MONITOR_COLUMN_KEYS)[keyof typeof MONITOR_COLUMN_KEYS]

type MonitorVisibleColumns = Record<MonitorColumnId, boolean>

type MonitorColumnDefinition = {
  key: MonitorColumnId
  label: string
  header: React.ReactNode
  headClassName?: string
  cellClassName?: string
  render: (record: MonitorRecord) => React.ReactNode
}

function getDefaultMonitorVisibleColumns(): MonitorVisibleColumns {
  return {
    [MONITOR_COLUMN_KEYS.TIME]: true,
    [MONITOR_COLUMN_KEYS.STATUS]: true,
    [MONITOR_COLUMN_KEYS.MODEL]: true,
    [MONITOR_COLUMN_KEYS.CHANNEL]: true,
    [MONITOR_COLUMN_KEYS.TOKEN_USAGE]: true,
    [MONITOR_COLUMN_KEYS.DURATION]: true,
    [MONITOR_COLUMN_KEYS.TTFT]: true,
    [MONITOR_COLUMN_KEYS.THROUGHPUT]: true,
  }
}

function getInitialMonitorVisibleColumns(): MonitorVisibleColumns {
  const defaults = getDefaultMonitorVisibleColumns()

  if (typeof localStorage === 'undefined') {
    return defaults
  }

  const savedColumns = localStorage.getItem(MONITOR_COLUMN_STORAGE_KEY)
  if (!savedColumns) {
    return defaults
  }

  try {
    return {
      ...defaults,
      ...JSON.parse(savedColumns),
    }
  } catch {
    return defaults
  }
}

function getStatusLabel(status: string, t: (key: string) => string): string {
  if (status === 'completed') return t('Completed')
  if (status === 'error') return t('Error')
  if (status === 'abandoned') return t('Failed')
  if (status === 'streaming') return t('Streaming')
  if (status === 'waiting_upstream') return t('Waiting')
  if (status === 'processing') return t('Running')
  if (status === 'pending') return t('Pending')
  return status || '-'
}

function getStatusClassName(status: string): string {
  if (status === 'completed') {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/70 dark:bg-emerald-950/40 dark:text-emerald-300'
  }
  if (status === 'error' || status === 'abandoned') {
    return 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900/70 dark:bg-rose-950/40 dark:text-rose-300'
  }
  if (status === 'streaming') {
    return 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/70 dark:bg-sky-950/40 dark:text-sky-300'
  }
  return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/70 dark:bg-amber-950/40 dark:text-amber-300'
}

function getAttemptStatusLabel(
  status: string | undefined,
  t: (key: string) => string
): string {
  if (status === 'waiting_upstream') return t('Waiting')
  if (status === 'streaming') return t('Streaming')
  if (status === 'failed') return t('Failed')
  if (status === 'abandoned') return t('Abandoned')
  if (status === 'succeeded') return t('Success')
  if (status === 'completed') return t('Completed')
  if (status === 'error') return t('Error')
  return status || t('Unknown')
}

function getAttemptStatusClassName(status: string | undefined): string {
  if (status === 'succeeded' || status === 'completed') {
    return getStatusClassName('completed')
  }
  if (status === 'failed' || status === 'error' || status === 'abandoned') {
    return getStatusClassName('error')
  }
  if (status === 'streaming') {
    return getStatusClassName('streaming')
  }
  return getStatusClassName('waiting_upstream')
}

function MetricCard(props: {
  label: string
  value: string | number
  description?: string
}) {
  return (
    <Card className='rounded-lg py-3' size='sm'>
      <CardContent className='px-3'>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
        <div className='mt-1 text-lg font-semibold tabular-nums'>
          {props.value}
        </div>
        {props.description ? (
          <div className='text-muted-foreground mt-1 truncate text-xs'>
            {props.description}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function TokenUsageBadge({ record }: { record: MonitorRecord }) {
  const displayStatus = deriveDisplayStatus(record)
  if (!isTerminalStatus(displayStatus)) {
    return <span className='text-muted-foreground'>-</span>
  }

  const tokenUsage = getMonitorTokenUsage(record)
  return (
    <Badge
      variant='outline'
      className='h-6 w-[8rem] justify-center gap-1 border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/70 dark:bg-sky-950/40 dark:text-sky-300'
    >
      <span className='inline-flex w-[3.25rem] items-center justify-end gap-1'>
        <ArrowUpFromLine className='size-3 opacity-70' />
        {formatTokenCount(tokenUsage.promptTokens)}
      </span>
      <span className='text-muted-foreground'>|</span>
      <span className='inline-flex w-[3.25rem] items-center justify-end gap-1'>
        <ArrowDownToLine className='size-3 opacity-70' />
        {formatTokenCount(tokenUsage.completionTokens)}
      </span>
    </Badge>
  )
}

function MonitorToolbar(props: {
  connected: boolean
  modelSearch: string
  isFullscreen: boolean
  columns: MonitorColumnDefinition[]
  visibleColumns: MonitorVisibleColumns
  onModelSearchChange: (value: string) => void
  onColumnVisibilityChange: (
    columnKey: MonitorColumnId,
    checked: boolean
  ) => void
  onSelectAllColumns: (checked: boolean) => void
  onResetColumns: () => void
  onReconnect: () => void
  onFullscreenToggle: () => void
}) {
  const { t } = useTranslation()
  const columnValues = Object.values(props.visibleColumns)
  const allColumnsVisible = columnValues.every(Boolean)
  const someColumnsVisible = columnValues.some(Boolean)

  return (
    <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
      <div className='relative min-w-0 flex-1 sm:w-72 sm:flex-none'>
        <Search className='text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2' />
        <Input
          value={props.modelSearch}
          onChange={(event) => props.onModelSearchChange(event.target.value)}
          placeholder={t('Model')}
          className='pl-8'
        />
      </div>
      <div className='flex items-center gap-2'>
        <Badge
          variant='outline'
          className={cn(
            'h-8 rounded-lg px-2.5',
            props.connected
              ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/70 dark:bg-emerald-950/40 dark:text-emerald-300'
              : 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900/70 dark:bg-rose-950/40 dark:text-rose-300'
          )}
        >
          {props.connected ? (
            <Wifi className='size-3.5' />
          ) : (
            <WifiOff className='size-3.5' />
          )}
          {props.connected ? t('Online') : t('Error')}
        </Badge>
        <DropdownMenu modal={false}>
          <Tooltip>
            <TooltipTrigger
              render={
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant='outline'
                      size='icon'
                      aria-label={t('Toggle columns')}
                    />
                  }
                />
              }
            >
              <Settings2 />
              <span className='sr-only'>{t('Toggle columns')}</span>
            </TooltipTrigger>
            <TooltipContent>{t('Toggle columns')}</TooltipContent>
          </Tooltip>
          <DropdownMenuContent align='end' className='w-52'>
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t('Toggle columns')}</DropdownMenuLabel>
              <DropdownMenuCheckboxItem
                checked={allColumnsVisible}
                aria-checked={
                  someColumnsVisible && !allColumnsVisible ? 'mixed' : undefined
                }
                onCheckedChange={props.onSelectAllColumns}
              >
                {t('Select all')}
              </DropdownMenuCheckboxItem>
              <DropdownMenuItem onSelect={props.onResetColumns}>
                {t('Reset')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              {props.columns.map((column) => (
                <DropdownMenuCheckboxItem
                  key={column.key}
                  checked={props.visibleColumns[column.key]}
                  onCheckedChange={(checked) =>
                    props.onColumnVisibilityChange(column.key, checked)
                  }
                >
                  {column.label}
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={props.onReconnect}
              />
            }
          >
            <RefreshCw />
            <span className='sr-only'>{t('Refresh')}</span>
          </TooltipTrigger>
          <TooltipContent>{t('Refresh')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={props.onFullscreenToggle}
              />
            }
          >
            {props.isFullscreen ? <Minimize2 /> : <Expand />}
            <span className='sr-only'>{t('Expand')}</span>
          </TooltipTrigger>
          <TooltipContent>{t('Expand')}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  )
}

function MonitorTable(props: {
  records: MonitorRecord[]
  selectedId: string | null
  columns: MonitorColumnDefinition[]
  onSelect: (record: MonitorRecord) => void
}) {
  const { t } = useTranslation()

  if (props.records.length === 0) {
    return (
      <div className='text-muted-foreground flex h-full min-h-48 items-center justify-center rounded-lg border border-dashed text-sm'>
        {t('No data')}
      </div>
    )
  }

  if (props.columns.length === 0) {
    return (
      <div className='text-muted-foreground flex h-full min-h-48 items-center justify-center rounded-lg border border-dashed text-sm'>
        {t('Toggle columns')}
      </div>
    )
  }

  return (
    <div className='h-full overflow-auto rounded-lg border'>
      <Table className='min-w-[980px]'>
        <TableHeader className='bg-muted/50 sticky top-0 z-10'>
          <TableRow>
            {props.columns.map((column) => (
              <TableHead key={column.key} className={column.headClassName}>
                {column.header}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.records.map((record) => (
            <TableRow
              key={record.id}
              className={cn(
                'cursor-pointer',
                props.selectedId === record.id && 'bg-muted/70'
              )}
              onClick={() => props.onSelect(record)}
            >
              {props.columns.map((column) => (
                <TableCell key={column.key} className={column.cellClassName}>
                  {column.render(record)}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function BodyTabs(props: {
  tabs: MonitorBodyType[]
  defaultValue: MonitorBodyType
  requestId: string
}) {
  const { t } = useTranslation()
  const [bodyTab, setBodyTab] = useState<MonitorBodyType>(props.defaultValue)

  useEffect(() => {
    setBodyTab(props.defaultValue)
  }, [props.defaultValue, props.requestId])

  return (
    <Tabs
      value={bodyTab}
      onValueChange={(value) => setBodyTab(value as MonitorBodyType)}
    >
      <TabsList
        className={cn(
          'grid w-full sm:w-72',
          props.tabs.length === 1 ? 'grid-cols-1' : 'grid-cols-2'
        )}
      >
        {props.tabs.map((type) => (
          <TabsTrigger key={type} value={type}>
            {type === 'response' ? t('Body') : t('Request')}
          </TabsTrigger>
        ))}
      </TabsList>
      {props.tabs.map((type) => (
        <TabsContent key={type} value={type} className='mt-3'>
          <BodyPanel requestId={props.requestId} type={type} />
        </TabsContent>
      ))}
    </Tabs>
  )
}

function DetailCard(props: {
  icon: React.ReactNode
  title: string
  children: React.ReactNode
}) {
  return (
    <Card className='rounded-lg py-0'>
      <CardContent className='p-3'>
        <div className='mb-3 flex items-center gap-2 text-sm font-semibold'>
          <span className='text-primary inline-flex'>{props.icon}</span>
          <span>{props.title}</span>
        </div>
        {props.children}
      </CardContent>
    </Card>
  )
}

function DetailPill(props: {
  icon: React.ReactNode
  label: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'bg-muted/35 inline-flex min-h-8 min-w-0 flex-[1_1_16rem] items-center gap-2 rounded-full px-3 py-1.5',
        props.className
      )}
    >
      <span className='text-muted-foreground inline-flex shrink-0'>
        {props.icon}
      </span>
      <span className='text-muted-foreground shrink-0 text-xs'>
        {props.label}
      </span>
      <span className='inline-flex min-w-0 items-center text-xs font-medium'>
        {props.children}
      </span>
    </div>
  )
}

function HeadersViewer(props: {
  headers?: Record<string, string>
  emptyLabel: string
}) {
  const entries = Object.entries(props.headers ?? {})
  if (entries.length === 0) {
    return (
      <span className='text-muted-foreground text-sm'>{props.emptyLabel}</span>
    )
  }

  return (
    <div className='bg-muted/30 space-y-1 rounded-md border p-3 font-mono text-xs'>
      {entries.map(([key, value]) => (
        <div key={key} className='min-w-0 break-all'>
          <span className='text-primary font-semibold'>{key}:</span>{' '}
          <span>{value}</span>
        </div>
      ))}
    </div>
  )
}

function BodyPanel(props: { requestId: string; type: MonitorBodyType }) {
  const { t } = useTranslation()
  const [body, setBody] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    void getMonitorBody(props.requestId, props.type)
      .then((response) => {
        if (cancelled) return
        if (response.success) {
          setBody(tryPrettyJson(response.body ?? ''))
        } else {
          setError(response.message || t('Request failed'))
          setBody('')
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : t('Request failed'))
          setBody('')
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [props.requestId, props.type, t])

  if (loading) {
    return (
      <div className='flex h-56 items-center justify-center'>
        <Spinner />
      </div>
    )
  }

  if (error) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>{t('Error')}</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }

  return (
    <Textarea
      readOnly
      value={body}
      className='h-80 resize-none font-mono text-xs leading-relaxed'
    />
  )
}

function RequestDetail(props: {
  record: MonitorRecord | null
  loading: boolean
  error: string | null
  interrupting: boolean
  onInterrupt: (
    id: string
  ) => Promise<{ success: boolean; error: string | null }>
}) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <div className='flex min-h-96 items-center justify-center rounded-lg border'>
        <Spinner />
      </div>
    )
  }

  if (props.error) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>{t('Error')}</AlertTitle>
        <AlertDescription>{props.error}</AlertDescription>
      </Alert>
    )
  }

  if (!props.record) {
    return (
      <div className='text-muted-foreground flex min-h-96 items-center justify-center rounded-lg border border-dashed text-sm'>
        {t('Details')}
      </div>
    )
  }

  const displayStatus = deriveDisplayStatus(props.record)
  const tokenUsage = getMonitorTokenUsage(props.record)
  const canInterrupt = isActiveStatus(displayStatus)
  const recordId = props.record.id
  const activeAttemptIndex = (props.record.channel_attempts ?? [])
    .map((attempt, index) => ({ attempt, index }))
    .reverse()
    .find(
      ({ attempt }) =>
        attempt.status === 'waiting_upstream' || attempt.status === 'streaming'
    )?.index

  const handleInterrupt = async () => {
    const result = await props.onInterrupt(recordId)
    if (result.success) {
      toast.success(t('Success'))
    } else {
      toast.error(result.error || t('Request failed'))
    }
  }

  return (
    <div className='space-y-3'>
      <DetailCard
        icon={<Network className='size-4' />}
        title={t('Current Channel')}
      >
        <div className='flex flex-wrap gap-2'>
          <DetailPill
            icon={<Route className='size-3.5' />}
            label={t('Channel')}
          >
            <span className='truncate'>
              {props.record.current_channel?.name ||
                props.record.channel_name ||
                '-'}
              {props.record.current_channel?.id
                ? ` / ID ${props.record.current_channel.id}`
                : ''}
              {props.record.current_channel?.attempt
                ? ` / ${t('Attempt {{num}}', {
                    num: props.record.current_channel.attempt,
                  })}`
                : ''}
            </span>
          </DetailPill>
          <DetailPill
            icon={<Activity className='size-3.5' />}
            label={t('Current Status')}
          >
            <Badge
              variant='outline'
              className={cn('h-6', getStatusClassName(displayStatus))}
            >
              {getStatusLabel(displayStatus, t)}
            </Badge>
          </DetailPill>
          {canInterrupt ? (
            <Button
              variant='destructive'
              size='sm'
              disabled={props.interrupting}
              onClick={handleInterrupt}
              className='ml-auto'
            >
              {props.interrupting ? <Spinner /> : <PauseCircle />}
              {t('Cancel')}
            </Button>
          ) : null}
        </div>

        <div className='mt-3 border-t pt-3'>
          <div className='mb-2 flex items-center justify-between gap-2'>
            <div className='flex items-center gap-1.5 text-sm font-medium'>
              <History className='text-muted-foreground size-3.5' />
              {t('Retry History')}
            </div>
            {props.record.channel_attempts?.length ? (
              <Badge variant='outline' className='h-5'>
                {props.record.channel_attempts.length}
              </Badge>
            ) : null}
          </div>
          {props.record.channel_attempts?.length ? (
            <div className='space-y-2'>
              {props.record.channel_attempts.map((attempt, index) => (
                <div
                  key={`${attempt.attempt ?? index}-${attempt.channel_id ?? '-'}-${attempt.started_at ?? ''}`}
                  className='bg-muted/30 rounded-lg border px-3 py-2'
                >
                  <div className='flex flex-wrap items-center justify-between gap-2'>
                    <div className='flex min-w-0 flex-wrap items-center gap-2'>
                      <Badge variant='outline' className='h-6'>
                        {t('Attempt {{num}}', {
                          num: attempt.attempt ?? index + 1,
                        })}
                      </Badge>
                      <span className='truncate text-sm'>
                        {attempt.channel_name || t('Unknown Channel')}
                        {attempt.channel_id
                          ? ` (ID: ${attempt.channel_id})`
                          : ''}
                      </span>
                      <Badge
                        variant='outline'
                        className={cn(
                          'h-6',
                          getAttemptStatusClassName(attempt.status)
                        )}
                      >
                        {getAttemptStatusLabel(attempt.status, t)}
                      </Badge>
                    </div>
                    {canInterrupt && activeAttemptIndex === index ? (
                      <Button
                        variant='destructive'
                        size='sm'
                        disabled={props.interrupting}
                        onClick={handleInterrupt}
                      >
                        {props.interrupting ? <Spinner /> : <PauseCircle />}
                        {t('Cancel')}
                      </Button>
                    ) : null}
                  </div>
                  <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs'>
                    <span>
                      {t('Started')}:{' '}
                      {formatDateTime(
                        attempt.started_at,
                        attempt.started_at_ms
                      )}
                    </span>
                    {attempt.ended_at || attempt.ended_at_ms ? (
                      <span>
                        {t('Ended')}:{' '}
                        {formatDateTime(attempt.ended_at, attempt.ended_at_ms)}
                      </span>
                    ) : null}
                    {attempt.reason ? (
                      <span>
                        {t('Reason')}: {attempt.reason}
                      </span>
                    ) : null}
                    {attempt.error_code ? (
                      <span>
                        {t('Error Code')}: {attempt.error_code}
                      </span>
                    ) : null}
                    {attempt.http_status ? (
                      <span>HTTP {attempt.http_status}</span>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className='text-muted-foreground text-sm'>
              {t('No retry records')}
            </div>
          )}
        </div>
      </DetailCard>

      <DetailCard icon={<Hash className='size-4' />} title={t('Request Info')}>
        <div className='flex flex-wrap gap-2'>
          <DetailPill
            icon={<Hash className='size-3.5' />}
            label={t('Request ID')}
            className='flex-[2_1_28rem] rounded-lg'
          >
            <span className='font-mono break-all'>{props.record.id}</span>
          </DetailPill>
          <DetailPill
            icon={<Network className='size-3.5' />}
            label={t('Model')}
          >
            <span className='truncate'>{props.record.model || '-'}</span>
          </DetailPill>
          <DetailPill
            icon={<Activity className='size-3.5' />}
            label={t('Status')}
          >
            <Badge
              variant='outline'
              className={cn('h-6', getStatusClassName(displayStatus))}
            >
              {getStatusLabel(displayStatus, t)}
            </Badge>
          </DetailPill>
          <DetailPill icon={<Radio className='size-3.5' />} label={t('Stream')}>
            {props.record.is_stream ? (
              <Badge variant='outline' className='h-6'>
                {t('Yes')}
              </Badge>
            ) : (
              <span>{t('No')}</span>
            )}
          </DetailPill>
          <DetailPill icon={<Clock3 className='size-3.5' />} label={t('Time')}>
            <span>
              {formatDateTime(
                props.record.start_time,
                props.record.start_time_ms
              )}
            </span>
          </DetailPill>
          <DetailPill
            icon={<Clock3 className='size-3.5' />}
            label={t('Duration')}
          >
            <span>
              {formatDuration(getDurationMs(props.record, Date.now()))}
            </span>
          </DetailPill>
          <DetailPill icon={<User className='size-3.5' />} label={t('User ID')}>
            <span>{props.record.user_id || '-'}</span>
          </DetailPill>
          <DetailPill
            icon={<KeyRound className='size-3.5' />}
            label={t('Token Name')}
          >
            <span className='truncate'>{props.record.token_name || '-'}</span>
          </DetailPill>
          <DetailPill
            icon={<ArrowUpFromLine className='size-3.5' />}
            label={t('Input Tokens')}
          >
            <span>{formatTokenCount(tokenUsage.promptTokens)}</span>
          </DetailPill>
          <DetailPill
            icon={<ArrowDownToLine className='size-3.5' />}
            label={t('Output Tokens')}
          >
            <span>{formatTokenCount(tokenUsage.completionTokens)}</span>
          </DetailPill>
        </div>
      </DetailCard>

      <DetailCard
        icon={<ArrowUpFromLine className='size-4' />}
        title={t('Downstream Request')}
      >
        <div className='mb-3 flex flex-wrap gap-2'>
          <DetailPill icon={<Route className='size-3.5' />} label={t('Path')}>
            <span className='truncate'>
              {props.record.downstream?.method || '-'}{' '}
              {props.record.downstream?.path || '-'}
            </span>
          </DetailPill>
          <DetailPill
            icon={<Globe2 className='size-3.5' />}
            label={t('Client IP')}
          >
            <span>{props.record.downstream?.client_ip || '-'}</span>
          </DetailPill>
          <DetailPill
            icon={<Hash className='size-3.5' />}
            label={t('Body Size')}
          >
            <span>{formatBytes(props.record.downstream?.body_size || 0)}</span>
          </DetailPill>
        </div>
        <Tabs defaultValue='headers'>
          <TabsList className='grid w-full grid-cols-2 sm:w-72'>
            <TabsTrigger value='headers'>{t('Headers')}</TabsTrigger>
            <TabsTrigger value='body'>{t('Body')}</TabsTrigger>
          </TabsList>
          <TabsContent value='headers' className='mt-3'>
            <HeadersViewer
              headers={props.record.downstream?.headers}
              emptyLabel={t('No headers')}
            />
          </TabsContent>
          <TabsContent value='body' className='mt-3'>
            <BodyPanel requestId={recordId} type='downstream' />
          </TabsContent>
        </Tabs>
      </DetailCard>

      {props.record.upstream ? (
        <DetailCard
          icon={<ArrowDownToLine className='size-4' />}
          title={t('Upstream Request')}
        >
          <div className='mb-3 flex flex-wrap gap-2'>
            <DetailPill icon={<Globe2 className='size-3.5' />} label={t('URL')}>
              <span className='truncate'>
                {props.record.upstream.url || '-'}
              </span>
            </DetailPill>
            <DetailPill
              icon={<Route className='size-3.5' />}
              label={t('Method')}
            >
              <span>{props.record.upstream.method || '-'}</span>
            </DetailPill>
            <DetailPill
              icon={<Hash className='size-3.5' />}
              label={t('Body Size')}
            >
              <span>{formatBytes(props.record.upstream.body_size || 0)}</span>
            </DetailPill>
          </div>
          <Tabs defaultValue='headers'>
            <TabsList className='grid w-full grid-cols-2 sm:w-72'>
              <TabsTrigger value='headers'>{t('Headers')}</TabsTrigger>
              <TabsTrigger value='body'>{t('Body')}</TabsTrigger>
            </TabsList>
            <TabsContent value='headers' className='mt-3'>
              <HeadersViewer
                headers={props.record.upstream.headers}
                emptyLabel={t('No headers')}
              />
            </TabsContent>
            <TabsContent value='body' className='mt-3'>
              <BodyPanel requestId={recordId} type='upstream' />
            </TabsContent>
          </Tabs>
        </DetailCard>
      ) : null}

      <DetailCard
        icon={<ArrowDownToLine className='size-4' />}
        title={t('Response')}
      >
        <div className='mb-3 flex flex-wrap gap-2'>
          <DetailPill
            icon={<Hash className='size-3.5' />}
            label={t('Status Code')}
          >
            <Badge
              variant='outline'
              className={cn(
                'h-6',
                Number(
                  props.record.response?.status_code || props.record.status_code
                ) >= 400
                  ? getStatusClassName('error')
                  : getStatusClassName('completed')
              )}
            >
              {props.record.response?.status_code ||
                props.record.status_code ||
                '-'}
            </Badge>
          </DetailPill>
          <DetailPill
            icon={<ArrowUpFromLine className='size-3.5' />}
            label={t('Input Tokens')}
          >
            <span>{formatTokenCount(tokenUsage.promptTokens)}</span>
          </DetailPill>
          <DetailPill
            icon={<ArrowDownToLine className='size-3.5' />}
            label={t('Output Tokens')}
          >
            <span>{formatTokenCount(tokenUsage.completionTokens)}</span>
          </DetailPill>
        </div>
        {props.record.response?.error && props.record.status !== 'abandoned' ? (
          <Alert variant='destructive' className='mb-3'>
            <AlertTitle>{t('Error')}</AlertTitle>
            <AlertDescription>
              {props.record.response.error.message ||
                props.record.response.error.code ||
                '-'}
            </AlertDescription>
          </Alert>
        ) : null}
        <Tabs defaultValue='headers'>
          <TabsList className='grid w-full grid-cols-2 sm:w-72'>
            <TabsTrigger value='headers'>{t('Headers')}</TabsTrigger>
            <TabsTrigger value='body'>{t('Body')}</TabsTrigger>
          </TabsList>
          <TabsContent value='headers' className='mt-3'>
            <HeadersViewer
              headers={props.record.response?.headers}
              emptyLabel={t('No headers')}
            />
          </TabsContent>
          <TabsContent value='body' className='mt-3'>
            <BodyTabs
              tabs={RESPONSE_BODY_TABS}
              defaultValue='response'
              requestId={recordId}
            />
          </TabsContent>
        </Tabs>
      </DetailCard>
    </div>
  )
}

function useFullscreenWakeLock(
  targetRef: React.RefObject<HTMLDivElement | null>
) {
  const [isFullscreen, setIsFullscreen] = useState(false)
  const wakeLockRef = useRef<WakeLockSentinel | null>(null)

  useEffect(() => {
    const handleChange = () => {
      setIsFullscreen(Boolean(document.fullscreenElement))
    }
    document.addEventListener('fullscreenchange', handleChange)
    handleChange()
    return () => document.removeEventListener('fullscreenchange', handleChange)
  }, [])

  useEffect(() => {
    if (!isFullscreen || !navigator.wakeLock?.request) return
    let cancelled = false
    void navigator.wakeLock
      .request('screen')
      .then((lock) => {
        if (cancelled) {
          void lock.release()
          return
        }
        wakeLockRef.current = lock
        lock.addEventListener('release', () => {
          wakeLockRef.current = null
        })
      })
      .catch(() => undefined)

    return () => {
      cancelled = true
      const lock = wakeLockRef.current
      wakeLockRef.current = null
      void lock?.release().catch(() => undefined)
    }
  }, [isFullscreen])

  const toggleFullscreen = useCallback(() => {
    if (document.fullscreenElement) {
      void document.exitFullscreen()
      return
    }
    void targetRef.current?.requestFullscreen({ navigationUI: 'hide' })
  }, [targetRef])

  return { isFullscreen, toggleFullscreen }
}

export function Monitor() {
  const { t } = useTranslation()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [modelSearch, setModelSearch] = useState('')
  const [clientNowMs, setClientNowMs] = useState(Date.now())
  const [visibleColumns, setVisibleColumns] = useState(
    getInitialMonitorVisibleColumns
  )
  const fullscreenRef = useRef<HTMLDivElement | null>(null)
  const { isFullscreen, toggleFullscreen } =
    useFullscreenWakeLock(fullscreenRef)
  const detail = useRequestDetail()
  const monitorWs = useMonitorWs({ focusedRequestId: selectedId })

  useEffect(() => {
    const timer = setInterval(() => setClientNowMs(Date.now()), 100)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    if (monitorWs.channelUpdate?.request_id) {
      detail.applyLiveUpdate(
        monitorWs.channelUpdate.request_id,
        monitorWs.channelUpdate
      )
    }
  }, [detail, monitorWs.channelUpdate])

  const records = useMemo(() => {
    const search = modelSearch.trim().toLowerCase()
    const filtered = search
      ? monitorWs.summaries.filter((record) =>
          (record.model || '').toLowerCase().includes(search)
        )
      : monitorWs.summaries
    return [...filtered].sort((a, b) => {
      const bActive = isActiveStatus(deriveDisplayStatus(b)) ? 1 : 0
      const aActive = isActiveStatus(deriveDisplayStatus(a)) ? 1 : 0
      if (bActive !== aActive) return bActive - aActive
      return (b.start_time_ms || 0) - (a.start_time_ms || 0)
    })
  }, [modelSearch, monitorWs.summaries])

  const selectedRecord =
    detail.selectedDetail ||
    monitorWs.summaries.find((record) => record.id === selectedId) ||
    null

  const monitorColumns = useMemo<MonitorColumnDefinition[]>(
    () => [
      {
        key: MONITOR_COLUMN_KEYS.TIME,
        label: t('Time'),
        header: t('Time'),
        headClassName: 'w-[9.5rem]',
        cellClassName: 'text-muted-foreground',
        render: (record) =>
          formatDateTime(record.start_time, record.start_time_ms),
      },
      {
        key: MONITOR_COLUMN_KEYS.STATUS,
        label: t('Status'),
        header: t('Status'),
        headClassName: 'w-[8rem]',
        render: (record) => {
          const displayStatus = deriveDisplayStatus(record)
          return (
            <Badge
              variant='outline'
              className={cn('h-6', getStatusClassName(displayStatus))}
            >
              {getStatusLabel(displayStatus, t)}
            </Badge>
          )
        },
      },
      {
        key: MONITOR_COLUMN_KEYS.MODEL,
        label: t('Model'),
        header: t('Model'),
        cellClassName: 'max-w-[18rem] truncate font-medium',
        render: (record) => record.model || '-',
      },
      {
        key: MONITOR_COLUMN_KEYS.CHANNEL,
        label: t('Channel'),
        header: t('Channel'),
        cellClassName: 'max-w-[16rem] truncate',
        render: (record) => {
          const retryCount = getRetryCount(record)
          return (
            <>
              <span>{record.channel_name || record.channel_id || '-'}</span>
              {retryCount > 0 ? (
                <Badge variant='outline' className='ml-2 h-5 px-1.5'>
                  +{retryCount}
                </Badge>
              ) : null}
            </>
          )
        },
      },
      {
        key: MONITOR_COLUMN_KEYS.TOKEN_USAGE,
        label: `${t('Input')} / ${t('Output')}`,
        header: `${t('Input')} / ${t('Output')}`,
        headClassName: 'w-[9rem]',
        render: (record) => <TokenUsageBadge record={record} />,
      },
      {
        key: MONITOR_COLUMN_KEYS.DURATION,
        label: t('Duration'),
        header: t('Duration'),
        headClassName: 'w-[7rem]',
        render: (record) => formatDuration(getDurationMs(record, clientNowMs)),
      },
      {
        key: MONITOR_COLUMN_KEYS.TTFT,
        label: 'TTFT',
        header: 'TTFT',
        headClassName: 'w-[6rem]',
        render: (record) => {
          const ttftMs = getTtftMs(record)
          return ttftMs ? `${ttftMs}ms` : '-'
        },
      },
      {
        key: MONITOR_COLUMN_KEYS.THROUGHPUT,
        label: t('Throughput'),
        header: t('Throughput'),
        headClassName: 'w-[7rem]',
        render: (record) => {
          const outputSpeed = getOutputSpeed(record, clientNowMs)
          return outputSpeed ? `${outputSpeed.toFixed(1)}/s` : '-'
        },
      },
    ],
    [clientNowMs, t]
  )

  const visibleMonitorColumns = useMemo(
    () => monitorColumns.filter((column) => visibleColumns[column.key]),
    [monitorColumns, visibleColumns]
  )

  useEffect(() => {
    if (typeof localStorage === 'undefined') {
      return
    }
    localStorage.setItem(
      MONITOR_COLUMN_STORAGE_KEY,
      JSON.stringify(visibleColumns)
    )
  }, [visibleColumns])

  const handleColumnVisibilityChange = useCallback(
    (columnKey: MonitorColumnId, checked: boolean) => {
      setVisibleColumns((previous) => ({
        ...previous,
        [columnKey]: checked,
      }))
    },
    []
  )

  const handleSelectAllColumns = useCallback((checked: boolean) => {
    const defaults = getDefaultMonitorVisibleColumns()
    setVisibleColumns(
      Object.fromEntries(
        Object.keys(defaults).map((key) => [key, checked])
      ) as MonitorVisibleColumns
    )
  }, [])

  const handleResetColumns = useCallback(() => {
    setVisibleColumns(getDefaultMonitorVisibleColumns())
  }, [])

  const handleSelect = (record: MonitorRecord) => {
    setSelectedId(record.id)
    setDetailOpen(true)
    void detail.fetchDetail(record.id)
  }

  return (
    <div
      ref={fullscreenRef}
      className={cn(
        'bg-background h-full min-h-0',
        isFullscreen && 'fixed inset-0 z-50 p-3'
      )}
    >
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Monitor')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <MonitorToolbar
            connected={monitorWs.connected}
            modelSearch={modelSearch}
            isFullscreen={isFullscreen}
            columns={monitorColumns}
            visibleColumns={visibleColumns}
            onModelSearchChange={setModelSearch}
            onColumnVisibilityChange={handleColumnVisibilityChange}
            onSelectAllColumns={handleSelectAllColumns}
            onResetColumns={handleResetColumns}
            onReconnect={monitorWs.reconnect}
            onFullscreenToggle={toggleFullscreen}
          />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-3'>
            <div className='grid shrink-0 grid-cols-2 gap-2 md:grid-cols-4'>
              <MetricCard label={t('Total')} value={monitorWs.stats.total} />
              <MetricCard label={t('Active')} value={monitorWs.stats.active} />
              <MetricCard
                label={t('Memory')}
                value={formatBytes(monitorWs.stats.memory)}
              />
              <MetricCard
                label={t('Requests')}
                value={`${monitorWs.stats.load.active_requests}/${monitorWs.stats.load.capacity}`}
                description={
                  monitorWs.stats.load.degraded ? t('Warning') : undefined
                }
              />
            </div>

            {monitorWs.stats.load.degraded ? (
              <Alert className='shrink-0 border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/70 dark:bg-amber-950/30 dark:text-amber-200'>
                <Timer className='size-4' />
                <AlertTitle>{t('Warning')}</AlertTitle>
                <AlertDescription>
                  {t('System Performance Monitoring')}
                </AlertDescription>
              </Alert>
            ) : null}

            <div className='min-h-0 flex-1'>
              <MonitorTable
                records={records}
                selectedId={selectedId}
                columns={visibleMonitorColumns}
                onSelect={handleSelect}
              />
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <Dialog
        open={detailOpen}
        onOpenChange={setDetailOpen}
        title={t('Request Details')}
        contentClassName='h-[92vh] max-w-[calc(100vw-1rem)] gap-3 p-3 sm:max-w-[92vw] sm:p-4'
        contentHeight='calc(92vh - 5rem)'
        bodyClassName='py-0'
      >
        <RequestDetail
          record={selectedRecord}
          loading={detail.loading}
          error={detail.error}
          interrupting={detail.interrupting}
          onInterrupt={detail.interruptRequest}
        />
      </Dialog>
    </div>
  )
}
