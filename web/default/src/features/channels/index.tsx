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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Expand, Minimize2, Settings2 } from 'lucide-react'
import { type RefObject, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelOps } from './api'
import { ChannelManualRefreshButton } from './components/channel-refresh-controls'
import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'

function useFullscreenWakeLock(targetRef: RefObject<HTMLDivElement | null>) {
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

export function Channels() {
  const { t } = useTranslation()
  const fullscreenRef = useRef<HTMLDivElement | null>(null)
  const { isFullscreen, toggleFullscreen } =
    useFullscreenWakeLock(fullscreenRef)
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const channelOpsQuery = useQuery({
    queryKey: ['channel-ops'],
    queryFn: getChannelOps,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })
  const retryTimes = channelOpsQuery.data?.data?.retry_times
  const retryLabel =
    typeof retryTimes === 'number' ? `${t('Max Retries')}: ${retryTimes}` : null
  let retryBadge = null
  if (retryLabel) {
    retryBadge = isRoot ? (
      <Tooltip>
        <TooltipTrigger
          render={
            <Badge
              variant='outline'
              className='shrink-0 cursor-pointer'
              aria-label={t('Retry Settings')}
              render={
                <Link
                  to='/system-settings/models/$section'
                  params={{ section: 'routing-reliability' }}
                />
              }
            />
          }
        >
          <span>{retryLabel}</span>
          <Settings2 data-icon='inline-end' />
        </TooltipTrigger>
        <TooltipContent>
          <p>{t('Retry Settings')}</p>
        </TooltipContent>
      </Tooltip>
    ) : (
      <Badge variant='outline' className='shrink-0'>
        {retryLabel}
      </Badge>
    )
  }

  return (
    <ChannelsProvider>
      <div
        ref={fullscreenRef}
        className={cn(
          'bg-background flex h-full min-h-0 flex-col',
          isFullscreen && 'fixed inset-0 z-50 p-3'
        )}
      >
        <SectionPageLayout fixedContent>
          <SectionPageLayout.Title>
            <span className='flex min-w-0 items-center gap-2'>
              <span className='truncate'>{t('Channels')}</span>
              {retryBadge}
            </span>
          </SectionPageLayout.Title>
          <SectionPageLayout.Actions>
            <ChannelsPrimaryButtons />
            <ChannelManualRefreshButton />
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type='button'
                    variant='outline'
                    size='icon-sm'
                    onClick={toggleFullscreen}
                  />
                }
              >
                {isFullscreen ? <Minimize2 /> : <Expand />}
                <span className='sr-only'>
                  {isFullscreen ? t('Exit fullscreen') : t('Enter fullscreen')}
                </span>
              </TooltipTrigger>
              <TooltipContent>
                {isFullscreen ? t('Exit fullscreen') : t('Enter fullscreen')}
              </TooltipContent>
            </Tooltip>
          </SectionPageLayout.Actions>
          <SectionPageLayout.Content>
            <ChannelsTable />
          </SectionPageLayout.Content>
        </SectionPageLayout>
      </div>

      <ChannelsDialogs />
    </ChannelsProvider>
  )
}
