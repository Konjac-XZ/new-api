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
import { Loader2, RefreshCw } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { useChannels } from './channels-provider'

const AUTO_REFRESH_ENABLED_STORAGE_KEY = 'channels:auto-refresh-enabled'
const AUTO_REFRESH_INTERVAL_STORAGE_KEY = 'channels:auto-refresh-interval'
const DEFAULT_AUTO_REFRESH_INTERVAL_SECONDS = 30
const AUTO_REFRESH_INTERVAL_SECONDS = [10, 30, 60, 120, 300] as const

function loadAutoRefreshEnabled(): boolean {
  if (typeof localStorage === 'undefined') {
    return false
  }
  return localStorage.getItem(AUTO_REFRESH_ENABLED_STORAGE_KEY) === 'true'
}

function loadAutoRefreshInterval(): number {
  if (typeof localStorage === 'undefined') {
    return DEFAULT_AUTO_REFRESH_INTERVAL_SECONDS
  }

  const stored = Number(localStorage.getItem(AUTO_REFRESH_INTERVAL_STORAGE_KEY))
  return AUTO_REFRESH_INTERVAL_SECONDS.includes(
    stored as (typeof AUTO_REFRESH_INTERVAL_SECONDS)[number]
  )
    ? stored
    : DEFAULT_AUTO_REFRESH_INTERVAL_SECONDS
}

function formatIntervalLabel(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`
  }
  return `${seconds / 60}m`
}

export function ChannelManualRefreshButton() {
  const { t } = useTranslation()
  const { refreshChannels } = useChannels()
  const [isRefreshing, setIsRefreshing] = useState(false)

  const handleRefresh = async () => {
    if (isRefreshing) {
      return
    }

    setIsRefreshing(true)
    try {
      await refreshChannels({ force: true })
    } catch {
      toast.error(t('Refresh failed'))
    } finally {
      setIsRefreshing(false)
    }
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type='button'
            variant='outline'
            size='icon'
            aria-label={t('Refresh')}
            onClick={handleRefresh}
            disabled={isRefreshing}
          />
        }
      >
        {isRefreshing ? <Loader2 className='animate-spin' /> : <RefreshCw />}
        <span className='sr-only'>{t('Refresh')}</span>
      </TooltipTrigger>
      <TooltipContent>{t('Refresh')}</TooltipContent>
    </Tooltip>
  )
}

export function ChannelAutoRefreshControl() {
  const { t } = useTranslation()
  const { refreshChannels, getAutoRefreshBlockReason } = useChannels()
  const [enabled, setEnabled] = useState(loadAutoRefreshEnabled)
  const [intervalSeconds, setIntervalSeconds] = useState(
    loadAutoRefreshInterval
  )
  const [remainingSeconds, setRemainingSeconds] = useState(intervalSeconds)
  const refreshChannelsRef = useRef(refreshChannels)
  const refreshingRef = useRef(false)

  useEffect(() => {
    refreshChannelsRef.current = refreshChannels
  }, [refreshChannels])

  useEffect(() => {
    localStorage.setItem(AUTO_REFRESH_ENABLED_STORAGE_KEY, String(enabled))
  }, [enabled])

  useEffect(() => {
    localStorage.setItem(
      AUTO_REFRESH_INTERVAL_STORAGE_KEY,
      String(intervalSeconds)
    )
  }, [intervalSeconds])

  useEffect(() => {
    if (!enabled) {
      setRemainingSeconds(intervalSeconds)
      return undefined
    }

    setRemainingSeconds(intervalSeconds)
    const timer = window.setInterval(() => {
      setRemainingSeconds((previous) => {
        if (previous > 1) {
          return previous - 1
        }

        if (!refreshingRef.current) {
          refreshingRef.current = true
          void refreshChannelsRef
            .current()
            .catch(() => undefined)
            .finally(() => {
              refreshingRef.current = false
            })
        }

        return intervalSeconds
      })
    }, 1000)

    return () => window.clearInterval(timer)
  }, [enabled, intervalSeconds])

  const blockReason = enabled ? getAutoRefreshBlockReason() : null
  let tooltip = t('Auto refresh')
  if (blockReason) {
    tooltip = t('Auto refresh paused')
  } else if (enabled) {
    tooltip = t('Next refresh in {{seconds}}s', {
      seconds: remainingSeconds,
    })
  }

  return (
    <div
      className='flex items-center gap-1.5'
      data-channel-auto-refresh-control
    >
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type='button'
              variant={enabled ? 'default' : 'outline'}
              size='sm'
              onClick={() => setEnabled((previous) => !previous)}
              aria-pressed={enabled}
              aria-label={t('Auto refresh')}
              className='gap-1.5 px-2'
            />
          }
        >
          <RefreshCw
            className={cn(
              'h-4 w-4',
              enabled && !blockReason && 'animate-spin [animation-duration:2s]'
            )}
          />
          <span className='hidden lg:inline'>{t('Auto refresh')}</span>
          {enabled && (
            <span className='text-primary-foreground/80 font-mono text-xs tabular-nums'>
              {remainingSeconds}s
            </span>
          )}
        </TooltipTrigger>
        <TooltipContent className='max-w-xs'>
          <div className='space-y-1'>
            <div>{tooltip}</div>
            <div className='text-muted-foreground text-xs'>
              {t(
                'Auto refresh pauses while channel edits, dialogs, selected rows, or focused inputs could conflict.'
              )}
            </div>
          </div>
        </TooltipContent>
      </Tooltip>

      <Select
        value={String(intervalSeconds)}
        onValueChange={(value) => {
          const next = Number(value)
          if (
            AUTO_REFRESH_INTERVAL_SECONDS.includes(
              next as (typeof AUTO_REFRESH_INTERVAL_SECONDS)[number]
            )
          ) {
            setIntervalSeconds(next)
          }
        }}
      >
        <SelectTrigger
          size='sm'
          className='w-[74px]'
          aria-label={t('Refresh interval')}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {AUTO_REFRESH_INTERVAL_SECONDS.map((seconds) => (
              <SelectItem key={seconds} value={String(seconds)}>
                {formatIntervalLabel(seconds)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}
