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
import { useCallback, useEffect, useRef, useState } from 'react'

const DESKTOP_POINTER_QUERY = '(hover: hover) and (pointer: fine)'

export function useFullscreen(
  targetRef: React.RefObject<HTMLDivElement | null>
) {
  const [isNativeFullscreen, setIsNativeFullscreen] = useState(false)
  const [isFocusMode, setIsFocusMode] = useState(false)
  const wakeLockRef = useRef<WakeLockSentinel | null>(null)
  const isFullscreen = isNativeFullscreen || isFocusMode

  useEffect(() => {
    const handleChange = () => {
      setIsNativeFullscreen(Boolean(document.fullscreenElement))
    }
    document.addEventListener('fullscreenchange', handleChange)
    handleChange()
    return () => document.removeEventListener('fullscreenchange', handleChange)
  }, [])

  useEffect(() => {
    if (!isFocusMode) return

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setIsFocusMode(false)
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [isFocusMode])

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
    if (isFocusMode) {
      setIsFocusMode(false)
      return
    }
    if (window.matchMedia?.(DESKTOP_POINTER_QUERY).matches) {
      setIsFocusMode(true)
      return
    }
    void targetRef.current?.requestFullscreen({ navigationUI: 'hide' })
  }, [isFocusMode, targetRef])

  return { isFullscreen, toggleFullscreen }
}
