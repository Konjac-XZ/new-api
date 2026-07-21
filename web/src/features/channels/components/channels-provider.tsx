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
/* eslint-disable react-refresh/only-export-components */
import { useQueryClient } from '@tanstack/react-query'
import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useMemo,
} from 'react'

import { useChannelUpstreamUpdates } from '../hooks/use-channel-upstream-updates'
import {
  channelsQueryKeys,
  loadStoredChannelSortRules,
  normalizeChannelSortRules,
  persistChannelSortRules,
} from '../lib'
import type { Channel, ChannelSortRule } from '../types'

// ============================================================================
// Types
// ============================================================================

type DialogType =
  | 'create-channel'
  | 'update-channel'
  | 'test-channel'
  | 'balance-query'
  | 'fetch-models'
  | 'ollama-models'
  | 'multi-key-manage'
  | 'breaker-status'
  | 'tag-batch-edit'
  | 'edit-tag'
  | 'copy-channel'
  | null

type UpstreamUpdateState = ReturnType<typeof useChannelUpstreamUpdates>

type AutoRefreshBlocker = 'row-selection'

type RefreshSkipReason =
  | 'active-input'
  | 'open-dialog'
  | 'channel-operation'
  | 'row-selection'

type RefreshChannelsOptions = {
  force?: boolean
}

type RefreshChannelsResult = {
  refreshed: boolean
  reason?: RefreshSkipReason
}

type ChannelsContextType = {
  open: DialogType
  setOpen: (open: DialogType) => void
  currentRow: Channel | null
  setCurrentRow: (row: Channel | null) => void
  currentTag: string | null
  setCurrentTag: (tag: string | null) => void
  enableTagMode: boolean
  setEnableTagMode: (enabled: boolean) => void
  channelSortRules: ChannelSortRule[]
  setChannelSortRules: (rules: ChannelSortRule[]) => void
  batchMode: boolean
  setBatchMode: (enabled: boolean) => void
  sensitiveVisible: boolean
  setSensitiveVisible: (visible: boolean) => void
  upstream: UpstreamUpdateState
  refreshChannels: (
    options?: RefreshChannelsOptions
  ) => Promise<RefreshChannelsResult>
  getAutoRefreshBlockReason: () => RefreshSkipReason | null
  setAutoRefreshBlocked: (blocker: AutoRefreshBlocker, blocked: boolean) => void
}

// ============================================================================
// Context
// ============================================================================

const ChannelsContext = createContext<ChannelsContextType | undefined>(
  undefined
)

function hasActiveTextInput(): boolean {
  if (typeof document === 'undefined') {
    return false
  }

  const activeElement = document.activeElement
  if (!(activeElement instanceof HTMLElement)) {
    return false
  }

  const tagName = activeElement.tagName.toLowerCase()
  if (tagName === 'input' || tagName === 'textarea' || tagName === 'select') {
    return true
  }

  if (activeElement.isContentEditable) {
    return true
  }

  const role = activeElement.getAttribute('role')
  return role === 'textbox' || role === 'spinbutton'
}

function hasVisibleBlockingOverlay(): boolean {
  if (typeof document === 'undefined') {
    return false
  }

  const overlays = document.querySelectorAll(
    '[data-slot="dialog-content"], [data-slot="drawer-content"], [role="dialog"]'
  )
  return [...overlays].some((overlay) => {
    if (!(overlay instanceof HTMLElement)) {
      return false
    }
    return overlay.getClientRects().length > 0
  })
}

// ============================================================================
// Provider
// ============================================================================

export function ChannelsProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState<DialogType>(null)
  const [currentRow, setCurrentRow] = useState<Channel | null>(null)
  const [currentTag, setCurrentTag] = useState<string | null>(null)
  const [enableTagMode, setEnableTagMode] = useState(() => {
    return localStorage.getItem('enable-tag-mode') === 'true'
  })
  const [channelSortRules, setChannelSortRulesState] = useState(
    loadStoredChannelSortRules
  )
  const [batchMode, setBatchMode] = useState(false)
  const [sensitiveVisible, setSensitiveVisible] = useState(true)
  const [autoRefreshBlockers, setAutoRefreshBlockers] = useState<
    AutoRefreshBlocker[]
  >([])

  const setChannelSortRules = useCallback((rules: ChannelSortRule[]) => {
    const normalized = normalizeChannelSortRules(rules)
    persistChannelSortRules(normalized)
    setChannelSortRulesState(normalized)
  }, [])

  const queryClient = useQueryClient()
  const refreshChannelLists = useCallback(async () => {
    await queryClient.invalidateQueries({
      queryKey: channelsQueryKeys.lists(),
    })
  }, [queryClient])
  const refreshAllChannels = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
  }, [queryClient])
  const upstream = useChannelUpstreamUpdates(refreshAllChannels)

  const getAutoRefreshBlockReason =
    useCallback((): RefreshSkipReason | null => {
      if (open !== null || upstream.showModal || hasVisibleBlockingOverlay()) {
        return 'open-dialog'
      }

      if (
        upstream.applyLoading ||
        upstream.detectAllLoading ||
        upstream.applyAllLoading
      ) {
        return 'channel-operation'
      }

      if (autoRefreshBlockers.includes('row-selection')) {
        return 'row-selection'
      }

      if (hasActiveTextInput()) {
        return 'active-input'
      }

      return null
    }, [
      open,
      upstream.showModal,
      upstream.applyLoading,
      upstream.detectAllLoading,
      upstream.applyAllLoading,
      autoRefreshBlockers,
    ])

  const refreshChannels = useCallback(
    async (
      options: RefreshChannelsOptions = {}
    ): Promise<RefreshChannelsResult> => {
      if (!options.force) {
        const reason = getAutoRefreshBlockReason()
        if (reason) {
          return { refreshed: false, reason }
        }
      }

      await refreshChannelLists()
      return { refreshed: true }
    },
    [getAutoRefreshBlockReason, refreshChannelLists]
  )

  const setAutoRefreshBlocked = useCallback(
    (blocker: AutoRefreshBlocker, blocked: boolean) => {
      setAutoRefreshBlockers((previous) => {
        const hasBlocker = previous.includes(blocker)
        if (blocked && !hasBlocker) {
          return [...previous, blocker]
        }
        if (!blocked && hasBlocker) {
          return previous.filter((item) => item !== blocker)
        }
        return previous
      })
    },
    []
  )
  // useState setters are stable, so the context value only needs to change when
  // an actual state value changes. Memoizing avoids handing every consumer
  // (including all channel cards/cells) a brand-new object on each render.
  const value = useMemo<ChannelsContextType>(
    () => ({
      open,
      setOpen,
      currentRow,
      setCurrentRow,
      currentTag,
      setCurrentTag,
      enableTagMode,
      setEnableTagMode,
      channelSortRules,
      setChannelSortRules,
      batchMode,
      setBatchMode,
      sensitiveVisible,
      setSensitiveVisible,
      upstream,
      refreshChannels,
      getAutoRefreshBlockReason,
      setAutoRefreshBlocked,
    }),
    [
      open,
      currentRow,
      currentTag,
      enableTagMode,
      channelSortRules,
      setChannelSortRules,
      batchMode,
      sensitiveVisible,
      upstream,
      refreshChannels,
      getAutoRefreshBlockReason,
      setAutoRefreshBlocked,
    ]
  )

  return (
    <ChannelsContext.Provider value={value}>
      {children}
    </ChannelsContext.Provider>
  )
}

// ============================================================================
// Hook
// ============================================================================

export function useChannels() {
  const context = useContext(ChannelsContext)
  if (!context) {
    throw new Error('useChannels must be used within ChannelsProvider')
  }
  return context
}
