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
import { getRouteApi } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  OnChangeFn,
  SortingState,
  Row,
} from '@tanstack/react-table'
import { Eye, EyeOff } from 'lucide-react'
import { useState, useMemo, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDebouncedColumnFilter,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { getLobeIcon } from '@/lib/lobe-icon'

import { getChannels, searchChannels, getGroups } from '../api'
import {
  DEFAULT_PAGE_SIZE,
  CHANNEL_STATUS,
  CHANNEL_STATUS_OPTIONS,
} from '../constants'
import {
  channelsQueryKeys,
  aggregateChannelsByTag,
  isTagAggregateRow,
  getChannelTypeIcon,
  getChannelTypeLabel,
  DEFAULT_CHANNEL_SORT_RULES,
  serializeChannelSortRules,
} from '../lib'
import type { Channel, ChannelSortBy, ChannelSortRule } from '../types'
import { ChannelCard } from './channel-card'
import { ChannelAutoRefreshControl } from './channel-refresh-controls'
import { useChannelsColumns } from './channels-columns'
import { useChannels } from './channels-provider'
import { DataTableBulkActions } from './data-table-bulk-actions'

const route = getRouteApi('/_authenticated/channels/')
const CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY = 'channels:column-visibility'
const CHANNELS_COLUMN_SIZING_STORAGE_KEY = 'channels:column-sizing'
const CHANNELS_VIEW_MODE_STORAGE_KEY = 'channels:view-mode'
const CHANNELS_STATUS_FILTER_STORAGE_KEY = 'channel-status-filter'
const CHANNELS_DYNAMIC_BREAKER_FILTER_STORAGE_KEY =
  'channel-dynamic-breaker-filter'

const CHANNEL_SORTABLE_COLUMNS = new Set<ChannelSortBy>([
  'id',
  'name',
  'priority',
  'weight',
  'balance',
  'response_time',
  'test_time',
])

function isDisabledChannelRow(channel: Channel) {
  return (
    !isTagAggregateRow(channel) && channel.status !== CHANNEL_STATUS.ENABLED
  )
}

function normalizeDynamicBreakerFilter(value: string | null | undefined) {
  if (value === 'enabled' || value === 'candidate' || value === 'active') {
    return value
  }
  return 'all'
}

function normalizeDynamicBreakerFilterValue(value: unknown) {
  const rawValue = Array.isArray(value) ? value[0] : value
  const normalized = normalizeDynamicBreakerFilter(
    typeof rawValue === 'string' ? rawValue : undefined
  )
  return normalized === 'all' ? [] : [normalized]
}

export function ChannelsTable() {
  const { t } = useTranslation()
  const {
    enableTagMode,
    channelSortRules,
    setChannelSortRules,
    batchMode,
    sensitiveVisible,
    setSensitiveVisible,
    setAutoRefreshBlocked,
  } = useChannels()
  const isMobile = useMediaQuery('(max-width: 640px)')

  // Table state
  const [sorting, setSorting] = useState<SortingState>([])

  // URL state management
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: {
      defaultPage: 1,
      defaultPageSize: DEFAULT_PAGE_SIZE,
    },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      {
        columnId: 'status',
        searchKey: 'status',
        type: 'array',
        deserialize: (value) => {
          if (value !== undefined) return value
          const stored = localStorage.getItem(
            CHANNELS_STATUS_FILTER_STORAGE_KEY
          )
          return stored === 'enabled' || stored === 'disabled' ? [stored] : []
        },
      },
      {
        columnId: 'dynamic_breaker',
        searchKey: 'dynamic_breaker',
        type: 'array',
        deserialize: (value) => {
          if (value !== undefined) {
            return normalizeDynamicBreakerFilterValue(value)
          }
          const stored = localStorage.getItem(
            CHANNELS_DYNAMIC_BREAKER_FILTER_STORAGE_KEY
          )
          return normalizeDynamicBreakerFilterValue(stored)
        },
      },
      { columnId: 'type', searchKey: 'type', type: 'array' },
      { columnId: 'group', searchKey: 'group', type: 'array' },
      { columnId: 'model', searchKey: 'model', type: 'string' },
    ],
  })

  const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (
    updater
  ) => {
    onColumnFiltersChange((previous) => {
      const next = typeof updater === 'function' ? updater(previous) : updater
      const status = next.find((f) => f.id === 'status')?.value as
        | string[]
        | undefined
      localStorage.setItem(
        CHANNELS_STATUS_FILTER_STORAGE_KEY,
        status?.[0] ?? 'all'
      )
      const dynamicBreaker = next.find((f) => f.id === 'dynamic_breaker')
        ?.value as string[] | undefined
      localStorage.setItem(
        CHANNELS_DYNAMIC_BREAKER_FILTER_STORAGE_KEY,
        dynamicBreaker?.[0] ?? 'all'
      )
      return next
    })
  }

  // Extract filters from column filters
  const statusFilter =
    (columnFilters.find((f) => f.id === 'status')?.value as string[]) || []
  const dynamicBreakerFilter =
    (columnFilters.find((f) => f.id === 'dynamic_breaker')?.value as
      | string[]
      | undefined) || []
  const dynamicBreakerMode =
    dynamicBreakerFilter.find(
      (value) =>
        value === 'enabled' || value === 'candidate' || value === 'active'
    ) ?? undefined
  const typeFilter = useMemo(
    () => (columnFilters.find((f) => f.id === 'type')?.value as string[]) || [],
    [columnFilters]
  )
  const groupFilter =
    (columnFilters.find((f) => f.id === 'group')?.value as string[]) || []
  const {
    value: modelFilter,
    inputValue: modelFilterInput,
    onChange: onModelFilterInputChange,
    onCompositionStart: onModelFilterCompositionStart,
    onCompositionEnd: onModelFilterCompositionEnd,
    resetInput: resetModelFilterInput,
  } = useDebouncedColumnFilter({
    columnFilters,
    columnId: 'model',
    onColumnFiltersChange,
  })

  // Determine whether to use search or regular list API
  const shouldSearch = Boolean(globalFilter?.trim() || modelFilter.trim())

  const sortRulesParam = useMemo(
    () => serializeChannelSortRules(channelSortRules),
    [channelSortRules]
  )

  const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
    setSorting((previous) => {
      const next = typeof updater === 'function' ? updater(previous) : updater
      const activeSort = next[0]
      if (!activeSort) {
        setChannelSortRules(DEFAULT_CHANNEL_SORT_RULES)
      } else if (CHANNEL_SORTABLE_COLUMNS.has(activeSort.id as ChannelSortBy)) {
        setChannelSortRules([
          {
            field: activeSort.id as ChannelSortBy,
            order: activeSort.desc ? 'desc' : 'asc',
          },
        ])
      }
      if (pagination.pageIndex > 0) {
        onPaginationChange({ ...pagination, pageIndex: 0 })
      }
      return next
    })
  }

  // Fetch groups for filter
  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
  })

  const groupOptions = useMemo(
    () =>
      (groupsData?.data || []).map((g) => ({
        label: g,
        value: g,
      })),
    [groupsData]
  )

  // Fetch channels data
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  const { data, isLoading, isFetching } = useQuery({
    queryKey: channelsQueryKeys.list({
      keyword: globalFilter,
      model: modelFilter,
      group:
        groupFilter.length > 0 && !groupFilter.includes('all')
          ? groupFilter[0]
          : undefined,
      status:
        statusFilter.length > 0 && !statusFilter.includes('all')
          ? statusFilter[0]
          : undefined,
      dynamic_breaker: dynamicBreakerMode,
      type:
        typeFilter.length > 0 && !typeFilter.includes('all')
          ? Number(typeFilter[0])
          : undefined,
      tag_mode: enableTagMode,
      sort_rules: sortRulesParam,
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    queryFn: async () => {
      if (shouldSearch) {
        return searchChannels({
          keyword: globalFilter,
          model: modelFilter,
          group:
            groupFilter.length > 0 && !groupFilter.includes('all')
              ? groupFilter[0]
              : undefined,
          status:
            statusFilter.length > 0 && !statusFilter.includes('all')
              ? statusFilter[0]
              : undefined,
          dynamic_breaker: dynamicBreakerMode,
          type:
            typeFilter.length > 0 && !typeFilter.includes('all')
              ? Number(typeFilter[0])
              : undefined,
          tag_mode: enableTagMode,
          sort_rules: sortRulesParam,
          p: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
        })
      } else {
        return getChannels({
          group:
            groupFilter.length > 0 && !groupFilter.includes('all')
              ? groupFilter[0]
              : undefined,
          status:
            statusFilter.length > 0 && !statusFilter.includes('all')
              ? statusFilter[0]
              : undefined,
          dynamic_breaker: dynamicBreakerMode,
          type:
            typeFilter.length > 0 && !typeFilter.includes('all')
              ? Number(typeFilter[0])
              : undefined,
          tag_mode: enableTagMode,
          sort_rules: sortRulesParam,
          p: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
        })
      }
    },
    placeholderData: (previousData) => previousData,
  })

  // Apply tag aggregation if tag mode is enabled
  const channels = useMemo(() => {
    const rawChannels = data?.data?.items || []

    if (enableTagMode && rawChannels.length > 0) {
      return aggregateChannelsByTag(rawChannels)
    }

    return rawChannels
  }, [data, enableTagMode])

  const totalCount = data?.data?.total || 0
  const typeCounts = data?.data?.type_counts

  // Columns configuration
  const columns = useChannelsColumns({ enableSelection: batchMode })

  // React Table instance
  const { table } = useDataTable({
    data: channels,
    columns,
    totalCount,
    sorting,
    initialColumnVisibility: {
      dynamic_breaker: false,
      models: false,
      tag: false,
    },
    columnVisibilityStorageKey: CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: isMobile
      ? false
      : CHANNELS_COLUMN_SIZING_STORAGE_KEY,
    columnFilters,
    pagination,
    globalFilter,
    enableRowSelection: batchMode
      ? (row: Row<Channel>) => !isTagAggregateRow(row.original)
      : false,
    onSortingChange: handleSortingChange,
    onColumnFiltersChange: handleColumnFiltersChange,
    onPaginationChange,
    onGlobalFilterChange,
    getSubRows: (row: Channel & { children?: Channel[] }) => row.children,
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    withExpandedRowModel: true,
    enableColumnResizing: !isMobile,
    ensurePageInRange,
  })

  useEffect(() => {
    if (!batchMode) {
      table.resetRowSelection()
    }
  }, [batchMode, table])

  const selectedRowCount = table.getSelectedRowModel().rows.length

  useEffect(() => {
    setAutoRefreshBlocked('row-selection', batchMode && selectedRowCount > 0)
  }, [batchMode, selectedRowCount, setAutoRefreshBlocked])

  useEffect(() => {
    const firstRule = channelSortRules[0] as ChannelSortRule | undefined
    if (!firstRule || !CHANNEL_SORTABLE_COLUMNS.has(firstRule.field)) {
      setSorting([])
      return
    }

    setSorting([{ id: firstRule.field, desc: firstRule.order === 'desc' }])
  }, [channelSortRules])

  // Prepare filter options from existing channel types only.
  const typeFilterOptions = useMemo(() => {
    const counts = typeCounts || {}
    const typeIds = Object.entries(counts)
      .map(([type, count]) => ({
        type: Number(type),
        count: Number(count) || 0,
      }))
      .filter((item) => item.type > 0 && item.count > 0)
      .sort((a, b) => {
        const labelA = t(getChannelTypeLabel(a.type))
        const labelB = t(getChannelTypeLabel(b.type))
        return labelA.localeCompare(labelB)
      })

    const selectedType = typeFilter.find((value) => value !== 'all')
    if (selectedType) {
      const selectedTypeId = Number(selectedType)
      const alreadyIncluded = typeIds.some(
        (item) => item.type === selectedTypeId
      )
      if (selectedTypeId > 0 && !alreadyIncluded) {
        typeIds.push({
          type: selectedTypeId,
          count: Number(counts[selectedType]) || 0,
        })
      }
    }

    const totalTypes = Object.values(counts).reduce(
      (sum, count) => sum + (Number(count) || 0),
      0
    )

    return [
      {
        label: 'All Types',
        value: 'all',
        count: totalTypes,
      },
      ...typeIds.map((item) => {
        const iconName = getChannelTypeIcon(item.type)
        return {
          label: getChannelTypeLabel(item.type),
          value: String(item.type),
          count: item.count,
          iconNode: getLobeIcon(`${iconName}.Color`, 16),
        }
      }),
    ]
  }, [t, typeCounts, typeFilter])

  const groupFilterOptions = [
    { label: t('All Groups'), value: 'all' },
    ...groupOptions.map((option) => ({
      ...option,
      label: sensitiveVisible ? option.label : '••••',
    })),
  ]

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Channels Found')}
      emptyDescription={t(
        'No channels available. Create your first channel to get started.'
      )}
      skeletonKeyPrefix='channel-skeleton'
      enableCardView
      viewModeStorageKey={CHANNELS_VIEW_MODE_STORAGE_KEY}
      renderCard={(row, { isSelected }) => (
        <ChannelCard row={row} isSelected={isSelected} />
      )}
      cardGridClassName='grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 xl:grid-cols-3'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Filter by name, ID, or key...'),
        searchClassName: 'hidden h-8 2xl:block 2xl:w-[200px]',
        searchDebounceMs: 500,
        hasAdditionalFilters: Boolean(modelFilterInput.trim()),
        onReset: () => {
          resetModelFilterInput()
        },
        additionalSearch: (
          <Input
            placeholder={t('Filter by model...')}
            aria-label={t('Filter by model...')}
            value={modelFilterInput}
            onChange={onModelFilterInputChange}
            onCompositionStart={onModelFilterCompositionStart}
            onCompositionEnd={onModelFilterCompositionEnd}
            className='hidden h-8 2xl:block 2xl:w-[160px]'
          />
        ),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            triggerClassName: 'h-8 w-[116px] shrink-0',
            options: [...CHANNEL_STATUS_OPTIONS],
            singleSelect: true,
            compactActiveIndicator: true,
          },
          {
            columnId: 'dynamic_breaker',
            title: t('Dynamic Breaker'),
            triggerClassName: 'h-8 w-[144px] shrink-0',
            options: [
              {
                value: 'enabled',
                label: 'Enabled',
              },
              {
                value: 'candidate',
                label: t('Candidate'),
              },
              {
                value: 'active',
                label: 'Active',
              },
            ],
            singleSelect: true,
            compactActiveIndicator: true,
          },
          {
            columnId: 'type',
            title: t('Type'),
            triggerClassName: 'h-8 w-[112px] shrink-0',
            options: typeFilterOptions,
            singleSelect: true,
            compactActiveIndicator: true,
          },
          {
            columnId: 'group',
            title: t('Group'),
            triggerClassName: 'h-8 w-[112px] shrink-0',
            options: groupFilterOptions,
            singleSelect: true,
            compactActiveIndicator: true,
          },
        ],
        preActions: (
          <>
            <ChannelAutoRefreshControl />
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon'
                    onClick={() => setSensitiveVisible(!sensitiveVisible)}
                    aria-label={sensitiveVisible ? t('Hide') : t('Show')}
                    className='text-muted-foreground hover:text-foreground'
                  />
                }
              >
                {sensitiveVisible ? <Eye /> : <EyeOff />}
              </TooltipTrigger>
              <TooltipContent>
                {sensitiveVisible ? t('Hide') : t('Show')}
              </TooltipContent>
            </Tooltip>
          </>
        ),
      }}
      getRowClassName={(row, { isMobile }) => {
        if (!isDisabledChannelRow(row.original)) {
          return undefined
        }
        if (isMobile) {
          return DISABLED_ROW_MOBILE
        }
        return DISABLED_ROW_DESKTOP
      }}
      bulkActions={batchMode ? <DataTableBulkActions table={table} /> : null}
    />
  )
}
