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
  ArrowDownToLine,
  ArrowUpFromLine,
  Expand,
  Minimize2,
  PauseCircle,
  RefreshCw,
  Search,
  Timer,
  Wifi,
  WifiOff,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
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

const BODY_TABS: MonitorBodyType[] = ['downstream', 'upstream', 'response']

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

function getBodyTabLabel(type: MonitorBodyType, t: (key: string) => string) {
  if (type === 'downstream') return t('Request')
  if (type === 'upstream') return t('Upstream')
  return t('Response')
}

function MonitorToolbar(props: {
  connected: boolean
  modelSearch: string
  isFullscreen: boolean
  onModelSearchChange: (value: string) => void
  onReconnect: () => void
  onFullscreenToggle: () => void
}) {
  const { t } = useTranslation()
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
  clientNowMs: number
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

  return (
    <div className='h-full overflow-auto rounded-lg border'>
      <Table className='min-w-[980px]'>
        <TableHeader className='bg-muted/50 sticky top-0 z-10'>
          <TableRow>
            <TableHead className='w-[9.5rem]'>{t('Time')}</TableHead>
            <TableHead className='w-[8rem]'>{t('Status')}</TableHead>
            <TableHead>{t('Model')}</TableHead>
            <TableHead>{t('Channel')}</TableHead>
            <TableHead className='w-[9rem]'>
              {t('Input')} / {t('Output')}
            </TableHead>
            <TableHead className='w-[7rem]'>{t('Duration')}</TableHead>
            <TableHead className='w-[6rem]'>TTFT</TableHead>
            <TableHead className='w-[7rem]'>{t('Throughput')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.records.map((record) => {
            const displayStatus = deriveDisplayStatus(record)
            const retryCount = getRetryCount(record)
            const ttftMs = getTtftMs(record)
            const outputSpeed = getOutputSpeed(record, props.clientNowMs)
            return (
              <TableRow
                key={record.id}
                className={cn(
                  'cursor-pointer',
                  props.selectedId === record.id && 'bg-muted/70'
                )}
                onClick={() => props.onSelect(record)}
              >
                <TableCell className='text-muted-foreground'>
                  {formatDateTime(record.start_time, record.start_time_ms)}
                </TableCell>
                <TableCell>
                  <Badge
                    variant='outline'
                    className={cn('h-6', getStatusClassName(displayStatus))}
                  >
                    {getStatusLabel(displayStatus, t)}
                  </Badge>
                </TableCell>
                <TableCell className='max-w-[18rem] truncate font-medium'>
                  {record.model || '-'}
                </TableCell>
                <TableCell className='max-w-[16rem] truncate'>
                  <span>{record.channel_name || record.channel_id || '-'}</span>
                  {retryCount > 0 ? (
                    <Badge variant='outline' className='ml-2 h-5 px-1.5'>
                      +{retryCount}
                    </Badge>
                  ) : null}
                </TableCell>
                <TableCell>
                  <TokenUsageBadge record={record} />
                </TableCell>
                <TableCell>
                  {formatDuration(getDurationMs(record, props.clientNowMs))}
                </TableCell>
                <TableCell>{ttftMs ? `${ttftMs}ms` : '-'}</TableCell>
                <TableCell>
                  {outputSpeed ? `${outputSpeed.toFixed(1)}/s` : '-'}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

function DetailMeta(props: { label: string; value: React.ReactNode }) {
  return (
    <div className='bg-muted/20 min-w-0 rounded-lg border px-3 py-2'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 truncate text-sm font-medium'>{props.value}</div>
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
      className='h-72 resize-none font-mono text-xs leading-relaxed'
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
  const [bodyTab, setBodyTab] = useState<MonitorBodyType>('downstream')

  if (props.loading) {
    return (
      <div className='flex h-full min-h-0 items-center justify-center rounded-lg border'>
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
      <div className='text-muted-foreground flex h-full min-h-0 items-center justify-center rounded-lg border border-dashed text-sm'>
        {t('Details')}
      </div>
    )
  }

  const displayStatus = deriveDisplayStatus(props.record)
  const tokenUsage = getMonitorTokenUsage(props.record)
  const canInterrupt = isActiveStatus(displayStatus)
  const recordId = props.record.id

  const handleInterrupt = async () => {
    const result = await props.onInterrupt(recordId)
    if (result.success) {
      toast.success(t('Success'))
    } else {
      toast.error(result.error || t('Request failed'))
    }
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      <div className='flex shrink-0 flex-wrap items-start justify-between gap-2'>
        <div className='min-w-0'>
          <div className='truncate text-sm font-semibold'>
            {props.record.id}
          </div>
          <div className='text-muted-foreground mt-1 truncate text-xs'>
            {props.record.model || '-'}
          </div>
        </div>
        <Button
          variant='destructive'
          size='sm'
          disabled={!canInterrupt || props.interrupting}
          onClick={handleInterrupt}
        >
          {props.interrupting ? <Spinner /> : <PauseCircle />}
          {t('Cancel')}
        </Button>
      </div>

      <div className='grid shrink-0 grid-cols-2 gap-2 xl:grid-cols-3'>
        <DetailMeta
          label={t('Status')}
          value={
            <Badge
              variant='outline'
              className={cn('h-6', getStatusClassName(displayStatus))}
            >
              {getStatusLabel(displayStatus, t)}
            </Badge>
          }
        />
        <DetailMeta
          label={t('Token Name')}
          value={props.record.token_name || '-'}
        />
        <DetailMeta
          label={t('Channel')}
          value={props.record.channel_name || '-'}
        />
        <DetailMeta
          label={t('Input Tokens')}
          value={formatTokenCount(tokenUsage.promptTokens)}
        />
        <DetailMeta
          label={t('Output Tokens')}
          value={formatTokenCount(tokenUsage.completionTokens)}
        />
        <DetailMeta
          label={t('IP')}
          value={props.record.downstream?.client_ip || '-'}
        />
      </div>

      <Tabs
        value={bodyTab}
        onValueChange={(value) => setBodyTab(value as MonitorBodyType)}
        className='flex min-h-0 flex-1 flex-col'
      >
        <TabsList className='shrink-0'>
          {BODY_TABS.map((type) => (
            <TabsTrigger key={type} value={type}>
              {getBodyTabLabel(type, t)}
            </TabsTrigger>
          ))}
        </TabsList>
        {BODY_TABS.map((type) => (
          <TabsContent key={type} value={type} className='min-h-0 flex-1'>
            <ScrollArea className='h-full'>
              <BodyPanel requestId={recordId} type={type} />
            </ScrollArea>
          </TabsContent>
        ))}
      </Tabs>
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
  const [modelSearch, setModelSearch] = useState('')
  const [clientNowMs, setClientNowMs] = useState(Date.now())
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

  const handleSelect = (record: MonitorRecord) => {
    setSelectedId(record.id)
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
            onModelSearchChange={setModelSearch}
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

            <div className='grid min-h-0 flex-1 gap-3 xl:grid-cols-[minmax(0,1.35fr)_minmax(420px,0.65fr)]'>
              <MonitorTable
                records={records}
                selectedId={selectedId}
                clientNowMs={clientNowMs}
                onSelect={handleSelect}
              />
              <Card className='min-h-0 rounded-lg py-0'>
                <CardContent className='h-full min-h-0 p-3'>
                  <RequestDetail
                    record={selectedRecord}
                    loading={detail.loading}
                    error={detail.error}
                    interrupting={detail.interrupting}
                    onInterrupt={detail.interruptRequest}
                  />
                </CardContent>
              </Card>
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    </div>
  )
}
