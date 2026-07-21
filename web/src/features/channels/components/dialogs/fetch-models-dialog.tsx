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
import { useQueryClient } from '@tanstack/react-query'
import { CheckCheck, Loader2, Search, Info, MinusCircle, X } from 'lucide-react'
import { useState, useEffect, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { tintedBadgeClassMap } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

import { fetchUpstreamModels, updateChannel } from '../../api'
import {
  buildCompactUpstreamModels,
  channelsQueryKeys,
  categorizeModelsWithRedirect,
  getReconciliationModelClassName,
  type CompactUpstreamModelEntry,
  normalizeModelName,
  parseModelsString,
} from '../../lib'
import { useChannels } from '../channels-provider'
import {
  ModelReconciliationList,
  type ModelReconciliationRow,
} from './model-reconciliation-list'

function normalizeModelNameList(models: readonly string[]): string[] {
  return [...new Set(models.map((m) => normalizeModelName(m)).filter(Boolean))]
}

function addNormalizedModel(models: string[], model: string): string[] {
  const normalized = normalizeModelName(model)
  if (!normalized) return models
  if (models.some((m) => normalizeModelName(m) === normalized)) return models
  return [...models, model]
}

function removeNormalizedModel(models: string[], model: string): string[] {
  const normalized = normalizeModelName(model)
  return models.filter((m) => normalizeModelName(m) !== normalized)
}

type ModelTab = 'new' | 'existing' | 'removed'

type FetchModelsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onModelsSelected?: (models: string[]) => void
  onCompactModelsSelected?: (data: {
    models: string[]
    modelMapping: string
    duplicateModels: string[]
    conflictModels: string[]
  }) => void
  compactMode?: boolean
  existingModelMapping?: string
  redirectModels?: string[]
  redirectSourceModels?: string[]
  customFetcher?: () => Promise<string[]>
  existingModelsOverride?: string[]
  channelName?: string | null
}

export function FetchModelsDialog({
  open,
  onOpenChange,
  onModelsSelected,
  onCompactModelsSelected,
  compactMode = false,
  existingModelMapping = '',
  redirectModels = [],
  redirectSourceModels = [],
  customFetcher,
  existingModelsOverride,
  channelName,
}: FetchModelsDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const activeChannel = customFetcher ? null : currentRow
  const queryClient = useQueryClient()
  const [isFetching, setIsFetching] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [rawFetchedModels, setRawFetchedModels] = useState<string[]>([])
  const [fetchedModels, setFetchedModels] = useState<string[]>([])
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [selectedCompactModels, setSelectedCompactModels] = useState<string[]>(
    []
  )
  const [selectedExistingModels, setSelectedExistingModels] = useState<
    string[]
  >([])
  const [searchKeyword, setSearchKeyword] = useState('')
  const [activeTab, setActiveTab] = useState<ModelTab>('existing')
  const [compactEntries, setCompactEntries] = useState<
    CompactUpstreamModelEntry[]
  >([])

  // Parse the latest form/channel value, then freeze it per dialog opening.
  const currentExistingModels = useMemo(
    () =>
      existingModelsOverride ?? parseModelsString(activeChannel?.models || ''),
    [existingModelsOverride, activeChannel?.models]
  )
  const [baseExistingModels, setBaseExistingModels] = useState<string[]>([])
  const existingModels = open ? baseExistingModels : currentExistingModels

  // Categorize models with redirect models
  const modelCategories = useMemo(
    () => categorizeModelsWithRedirect(existingModels, redirectModels),
    [existingModels, redirectModels]
  )

  const { classificationSet, redirectOnlySet } = modelCategories

  const fetchedModelSet = useMemo(
    () => new Set(normalizeModelNameList(fetchedModels)),
    [fetchedModels]
  )
  const rawFetchedModelSet = useMemo(
    () => new Set(normalizeModelNameList(rawFetchedModels)),
    [rawFetchedModels]
  )
  const compactEntryMap = useMemo(
    () =>
      new Map(
        compactEntries.map((entry) => [normalizeModelName(entry.model), entry])
      ),
    [compactEntries]
  )

  // Source keys in model_mapping are aliases, not real upstream IDs, so we
  // must skip them when computing "removed upstream" entries to avoid false
  // positives.
  const redirectSourceKeysSet = useMemo(
    () => new Set(normalizeModelNameList(redirectSourceModels)),
    [redirectSourceModels]
  )
  const redirectTargetBySourceModel = useMemo(() => {
    const trimmed = existingModelMapping.trim()
    if (!trimmed) return new Map<string, string>()

    try {
      const parsed = JSON.parse(trimmed)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return new Map<string, string>()
      }

      const entries = Object.entries(parsed)
        .map(([source, target]) => [
          normalizeModelName(source),
          typeof target === 'string' ? normalizeModelName(target) : '',
        ])
        .filter(
          (entry): entry is [string, string] =>
            Boolean(entry[0]) && Boolean(entry[1])
        )
      return new Map(entries)
    } catch {
      return new Map<string, string>()
    }
  }, [existingModelMapping])

  const removedModels = useMemo(() => {
    if (compactMode) return []
    const kw = searchKeyword.toLowerCase().trim()
    return existingModels.filter((model) => {
      const normalized = normalizeModelName(model)
      if (!normalized) return false
      if (fetchedModelSet.has(normalized)) return false
      const mappedTarget = redirectTargetBySourceModel.get(normalized)
      if (mappedTarget && fetchedModelSet.has(mappedTarget)) return false
      if (!mappedTarget && redirectSourceKeysSet.has(normalized)) return false
      if (!kw) return true
      return normalized.toLowerCase().includes(kw)
    })
  }, [
    compactMode,
    existingModels,
    fetchedModelSet,
    redirectTargetBySourceModel,
    redirectSourceKeysSet,
    searchKeyword,
  ])

  useEffect(() => {
    if (open && (activeChannel || customFetcher)) {
      const nextExistingModels = currentExistingModels
      setBaseExistingModels(nextExistingModels)
      handleFetchModels(nextExistingModels)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, activeChannel?.id, customFetcher])

  const handleFetchModels = async (baseModels = existingModels) => {
    if (!activeChannel && !customFetcher) return

    setIsFetching(true)
    try {
      if (customFetcher) {
        const list = await customFetcher()
        if (applyFetchedModels(list, baseModels)) {
          toast.success(t('Fetched {{count}} models', { count: list.length }))
        }
      } else {
        if (!activeChannel) return
        const response = await fetchUpstreamModels(activeChannel.id)
        if (response.success) {
          const list = Array.isArray(response.data) ? response.data : []
          if (applyFetchedModels(list, baseModels)) {
            toast.success(t('Fetched {{count}} models', { count: list.length }))
          }
        } else {
          toast.error(response.message || t('Failed to fetch models'))
          setFetchedModels([])
        }
      }
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to fetch models')
      )
      setFetchedModels([])
    } finally {
      setIsFetching(false)
    }
  }

  const applyFetchedModels = (
    list: string[],
    baseModels = existingModels
  ): boolean => {
    setRawFetchedModels(list)
    const baseCategories = categorizeModelsWithRedirect(
      baseModels,
      redirectModels
    )
    if (!compactMode) {
      const newFetchedModels = list.filter(
        (model) =>
          !baseCategories.classificationSet.has(normalizeModelName(model))
      )
      setFetchedModels(list)
      setSelectedModels(baseModels)
      setSelectedCompactModels([])
      setSelectedExistingModels([])
      setActiveTab(newFetchedModels.length > 0 ? 'new' : 'existing')
      setCompactEntries([])
      return true
    }

    const compactResult = buildCompactUpstreamModels({
      upstreamModels: list,
      existingModels: baseModels,
      existingModelMapping,
    })

    if (!compactResult.success) {
      toast.error(t(compactResult.error))
      setFetchedModels([])
      setSelectedModels([])
      setSelectedCompactModels([])
      setSelectedExistingModels([])
      setCompactEntries([])
      return false
    }

    setFetchedModels(compactResult.compactModels)
    setSelectedModels([])
    setSelectedCompactModels(compactResult.compactModels)
    setSelectedExistingModels(baseModels)
    const baseExistingSet = new Set(normalizeModelNameList(baseModels))
    setActiveTab(
      compactResult.entries.some(
        (entry) =>
          !baseExistingSet.has(normalizeModelName(entry.model)) &&
          !baseCategories.classificationSet.has(
            normalizeModelName(entry.upstreamModel)
          )
      )
        ? 'new'
        : 'existing'
    )
    setCompactEntries(compactResult.entries)
    return true
  }

  const handleSave = async () => {
    if (compactMode && onCompactModelsSelected) {
      const compactResult = buildCompactUpstreamModels({
        upstreamModels: rawFetchedModels,
        existingModels,
        existingModelMapping,
        selectedCompactModels,
        selectedExistingModels,
      })

      if (!compactResult.success) {
        toast.error(t(compactResult.error))
        return
      }

      onCompactModelsSelected({
        models: compactResult.models,
        modelMapping: compactResult.modelMapping,
        duplicateModels: compactResult.duplicateModels,
        conflictModels: compactResult.conflictModels,
      })
      toast.success(t('Compact models filled to form'))
      onOpenChange(false)
      return
    }

    // If onModelsSelected callback is provided, use it (form filling mode)
    if (onModelsSelected) {
      onModelsSelected(selectedModels)
      toast.success(t('Models filled to form'))
      onOpenChange(false)
      return
    }

    // Otherwise, directly save to API (standalone mode)
    if (!activeChannel) return
    setIsSaving(true)
    try {
      const modelsString = selectedModels.join(',')
      const response = await updateChannel(activeChannel.id, {
        models: modelsString,
      })
      if (response.success) {
        toast.success(t('Models updated successfully'))
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
        onOpenChange(false)
      } else {
        toast.error(response.message || t('Failed to update models'))
      }
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to update models')
      )
    } finally {
      setIsSaving(false)
    }
  }

  const handleClose = () => {
    setRawFetchedModels([])
    setFetchedModels([])
    setSelectedModels([])
    setSelectedCompactModels([])
    setSelectedExistingModels([])
    setCompactEntries([])
    setBaseExistingModels([])
    setSearchKeyword('')
    setActiveTab('existing')
    onOpenChange(false)
  }

  // Categorize models by common prefixes
  const categorizeModels = (models: string[]) => {
    const categories: Record<string, string[]> = {}

    models.forEach((model) => {
      let category = 'Other'

      // Determine category based on model name
      if (
        model.toLowerCase().includes('gpt') ||
        model.toLowerCase().includes('o1') ||
        model.toLowerCase().includes('o3')
      ) {
        category = 'OpenAI'
      } else if (model.toLowerCase().includes('claude')) {
        category = 'Anthropic'
      } else if (model.toLowerCase().includes('gemini')) {
        category = 'Gemini'
      } else if (model.toLowerCase().includes('qwen')) {
        category = 'Qwen'
      } else if (model.toLowerCase().includes('deepseek')) {
        category = 'DeepSeek'
      } else if (model.toLowerCase().includes('glm')) {
        category = 'Zhipu'
      } else if (model.toLowerCase().includes('llama')) {
        category = 'Meta'
      } else if (model.toLowerCase().includes('mistral')) {
        category = 'Mistral'
      }

      if (!categories[category]) {
        categories[category] = []
      }
      categories[category].push(model)
    })

    return categories
  }

  // Filter models by search
  const filteredModels = useMemo(() => {
    if (!searchKeyword) return fetchedModels
    return fetchedModels.filter((model) =>
      model.toLowerCase().includes(searchKeyword.toLowerCase())
    )
  }, [fetchedModels, searchKeyword])
  const selectedModelSet = useMemo(
    () => new Set(normalizeModelNameList(selectedModels)),
    [selectedModels]
  )
  const selectedCompactModelSet = useMemo(
    () => new Set(normalizeModelNameList(selectedCompactModels)),
    [selectedCompactModels]
  )
  const selectedExistingModelSet = useMemo(
    () => new Set(normalizeModelNameList(selectedExistingModels)),
    [selectedExistingModels]
  )
  const existingModelSet = useMemo(
    () => new Set(normalizeModelNameList(existingModels)),
    [existingModels]
  )
  const compactEntryByUpstreamModel = useMemo(
    () =>
      new Map(
        compactEntries.map((entry) => [
          normalizeModelName(entry.upstreamModel),
          entry,
        ])
      ),
    [compactEntries]
  )
  const selectedCompactMappedTargetSet = useMemo(
    () =>
      new Set(
        compactEntries
          .filter(
            (entry) =>
              entry.shouldMap &&
              selectedCompactModelSet.has(normalizeModelName(entry.model))
          )
          .map((entry) => normalizeModelName(entry.upstreamModel))
      ),
    [compactEntries, selectedCompactModelSet]
  )
  const allRemovedModels = useMemo(() => {
    if (compactMode) return []
    return existingModels.filter((model) => {
      const normalized = normalizeModelName(model)
      if (!normalized) return false
      if (fetchedModelSet.has(normalized)) return false
      const mappedTarget = redirectTargetBySourceModel.get(normalized)
      if (mappedTarget && fetchedModelSet.has(mappedTarget)) return false
      if (!mappedTarget && redirectSourceKeysSet.has(normalized)) return false
      return true
    })
  }, [
    compactMode,
    existingModels,
    fetchedModelSet,
    redirectSourceKeysSet,
    redirectTargetBySourceModel,
  ])

  // Helper to check if a model is considered "existing" (in selected or redirect)
  const isExistingModel = (model: string) =>
    classificationSet.has(normalizeModelName(model))

  // Separate new and existing models
  const allNewModels = filteredModels.filter((m) => !isExistingModel(m))
  const allExistingFilteredModels = filteredModels.filter((m) =>
    isExistingModel(m)
  )
  const allFetchedNewModels = fetchedModels.filter((m) => !isExistingModel(m))
  const allFetchedExistingModels = fetchedModels.filter((m) =>
    isExistingModel(m)
  )
  const matchesSearch = useCallback(
    (model: string, relatedModel?: string) => {
      const kw = searchKeyword.toLowerCase().trim()
      if (!kw) return true
      if (model.toLowerCase().includes(kw)) return true
      return relatedModel?.toLowerCase().includes(kw) ?? false
    },
    [searchKeyword]
  )
  const compactNewRowsAll = useMemo<ModelReconciliationRow[]>(
    () =>
      compactEntries
        .filter(
          (entry) =>
            !classificationSet.has(normalizeModelName(entry.model)) &&
            !classificationSet.has(normalizeModelName(entry.upstreamModel))
        )
        .map((entry) => {
          const checked = selectedCompactModelSet.has(
            normalizeModelName(entry.model)
          )
          return {
            id: `compact-new:${entry.model}`,
            model: entry.model,
            checked,
            className: getReconciliationModelClassName({
              existedLocally: existingModelSet.has(
                normalizeModelName(entry.model)
              ),
              selected: checked,
            }),
            badgeLabel: entry.shouldMap
              ? t('Compacts {{model}}', { model: entry.upstreamModel })
              : t('From upstream'),
            badgeClassName: entry.shouldMap
              ? tintedBadgeClassMap.success
              : tintedBadgeClassMap.neutral,
          }
        }),
    [
      classificationSet,
      compactEntries,
      existingModelSet,
      selectedCompactModelSet,
      t,
    ]
  )
  const compactExistingRowsAll = useMemo<ModelReconciliationRow[]>(
    () =>
      existingModels
        .filter((model) => {
          const normalized = normalizeModelName(model)
          if (!normalized) return false
          if (fetchedModelSet.has(normalized)) return true
          if (compactEntryMap.has(normalized)) return true
          const mappedTarget = redirectTargetBySourceModel.get(normalized)
          if (mappedTarget && rawFetchedModelSet.has(mappedTarget)) return true
          return compactEntryByUpstreamModel.has(normalized)
        })
        .map((model) => {
          const normalized = normalizeModelName(model)
          const compactEntry = compactEntryByUpstreamModel.get(normalized)
          const compactModelSelected =
            !!compactEntry &&
            selectedCompactModelSet.has(normalizeModelName(compactEntry.model))
          const compacted =
            compactEntry && selectedCompactMappedTargetSet.has(normalized)
          const existingCompactName =
            compactEntryMap.has(normalized) &&
            selectedCompactModelSet.has(normalized)
          let badgeLabel = t('Kept, not compacted')
          if (compacted) {
            badgeLabel = t('Compacted to {{model}}', {
              model: compactEntry.model,
            })
          } else if (compactEntry && !compactModelSelected) {
            badgeLabel = t('Compact not applied')
          } else if (existingCompactName) {
            badgeLabel = t('Existing compact name')
          }
          const checked =
            selectedExistingModelSet.has(normalized) &&
            !selectedCompactMappedTargetSet.has(normalized)

          return {
            id: `existing:${model}`,
            model,
            checked,
            className: getReconciliationModelClassName({
              existedLocally: true,
              selected: checked,
            }),
            badgeLabel,
            badgeClassName: compacted
              ? tintedBadgeClassMap.warning
              : tintedBadgeClassMap.neutral,
          }
        }),
    [
      compactEntryByUpstreamModel,
      compactEntryMap,
      existingModels,
      fetchedModelSet,
      rawFetchedModelSet,
      redirectTargetBySourceModel,
      selectedCompactMappedTargetSet,
      selectedCompactModelSet,
      selectedExistingModelSet,
      t,
    ]
  )
  const compactRemovedRowsAll = useMemo<ModelReconciliationRow[]>(
    () =>
      existingModels
        .map((model): ModelReconciliationRow | null => {
          const normalized = normalizeModelName(model)
          if (!normalized) return null
          if (fetchedModelSet.has(normalized)) return null
          if (compactEntryMap.has(normalized)) return null
          if (compactEntryByUpstreamModel.has(normalized)) return null
          const mappedTarget = redirectTargetBySourceModel.get(normalized)
          if (mappedTarget && rawFetchedModelSet.has(mappedTarget)) return null
          if (!mappedTarget && redirectSourceKeysSet.has(normalized)) {
            return null
          }

          return {
            id: `removed:${model}`,
            model,
            checked: selectedExistingModelSet.has(normalized),
            className: getReconciliationModelClassName({
              existedLocally: true,
              selected: selectedExistingModelSet.has(normalized),
            }),
            badgeLabel: t('Not returned by upstream'),
            badgeClassName: tintedBadgeClassMap.danger,
          }
        })
        .filter((row): row is ModelReconciliationRow => row !== null),
    [
      compactEntryByUpstreamModel,
      compactEntryMap,
      existingModels,
      fetchedModelSet,
      rawFetchedModelSet,
      redirectSourceKeysSet,
      redirectTargetBySourceModel,
      selectedExistingModelSet,
      t,
    ]
  )
  const compactRemovalPreviewRowsAll = useMemo<ModelReconciliationRow[]>(
    () =>
      existingModels
        .map((model): ModelReconciliationRow | null => {
          const normalized = normalizeModelName(model)
          const compactEntry = compactEntryByUpstreamModel.get(normalized)
          const compacted =
            compactEntry && selectedCompactMappedTargetSet.has(normalized)
          const manuallyRemoved = !selectedExistingModelSet.has(normalized)

          if (!compacted && !manuallyRemoved) return null

          return {
            id: `preview-removed:${model}`,
            model,
            checked: false,
            className: 'font-medium text-destructive',
            badgeLabel: compacted
              ? t('Compacted to {{model}}', { model: compactEntry.model })
              : t('Manual removal'),
            badgeClassName: compacted
              ? tintedBadgeClassMap.warning
              : tintedBadgeClassMap.danger,
          }
        })
        .filter((row): row is ModelReconciliationRow => row !== null),
    [
      compactEntryByUpstreamModel,
      existingModels,
      selectedCompactMappedTargetSet,
      selectedExistingModelSet,
      t,
    ]
  )
  const compactNewRows = useMemo(
    () =>
      compactNewRowsAll.filter((row) => {
        const entry = compactEntryMap.get(normalizeModelName(row.model))
        return matchesSearch(row.model, entry?.upstreamModel)
      }),
    [compactEntryMap, compactNewRowsAll, matchesSearch]
  )
  const compactExistingRows = useMemo(
    () =>
      compactExistingRowsAll.filter((row) => {
        const entry = compactEntryByUpstreamModel.get(
          normalizeModelName(row.model)
        )
        return matchesSearch(row.model, entry?.model)
      }),
    [compactEntryByUpstreamModel, compactExistingRowsAll, matchesSearch]
  )
  const compactRemovedRows = useMemo(
    () =>
      compactRemovedRowsAll.filter((row) => {
        const entry = compactEntryByUpstreamModel.get(
          normalizeModelName(row.model)
        )
        return matchesSearch(row.model, entry?.model)
      }),
    [compactEntryByUpstreamModel, compactRemovedRowsAll, matchesSearch]
  )
  const activeTabModels = useMemo(() => {
    if (activeTab === 'new') return allNewModels
    if (activeTab === 'removed') return removedModels
    return allExistingFilteredModels
  }, [activeTab, allExistingFilteredModels, allNewModels, removedModels])
  const activeCompactRows = useMemo(() => {
    if (activeTab === 'new') return compactNewRows
    if (activeTab === 'removed') return compactRemovedRows
    return compactExistingRows
  }, [activeTab, compactExistingRows, compactNewRows, compactRemovedRows])
  const selectedActiveTabCount = activeTabModels.filter((m) =>
    selectedModels.includes(m)
  ).length
  const selectedVisibleCount = activeTabModels.filter((m) =>
    selectedModels.includes(m)
  ).length
  const selectedActiveCompactCount = activeCompactRows.filter(
    (row) => row.checked
  ).length
  const activeVisibleCount = compactMode
    ? activeCompactRows.length
    : activeTabModels.length
  const selectedActiveVisibleCount = compactMode
    ? selectedActiveCompactCount
    : selectedActiveTabCount
  const selectedVisibleModelsCount = compactMode
    ? selectedActiveCompactCount
    : selectedVisibleCount
  const compactSelectedModelCount = useMemo(
    () =>
      new Set(
        [
          ...existingModels.filter((model) => {
            const normalized = normalizeModelName(model)
            return (
              selectedExistingModelSet.has(normalized) &&
              !selectedCompactMappedTargetSet.has(normalized)
            )
          }),
          ...compactEntries
            .filter((entry) =>
              selectedCompactModelSet.has(normalizeModelName(entry.model))
            )
            .map((entry) => entry.model),
        ].map((model) => normalizeModelName(model))
      ).size,
    [
      compactEntries,
      existingModels,
      selectedCompactMappedTargetSet,
      selectedCompactModelSet,
      selectedExistingModelSet,
    ]
  )

  const modelsToAdd = useMemo(
    () =>
      compactMode
        ? compactEntries
            .filter(
              (entry) =>
                selectedCompactModelSet.has(normalizeModelName(entry.model)) &&
                !existingModelSet.has(normalizeModelName(entry.model))
            )
            .map((entry) => entry.model)
        : selectedModels.filter(
            (model) => !existingModelSet.has(normalizeModelName(model))
          ),
    [
      compactEntries,
      compactMode,
      existingModelSet,
      selectedCompactModelSet,
      selectedModels,
    ]
  )
  const modelsToRemove = useMemo(
    () =>
      compactMode
        ? compactRemovalPreviewRowsAll.map((row) => row.model)
        : existingModels.filter(
            (model) => !selectedModelSet.has(normalizeModelName(model))
          ),
    [
      compactMode,
      compactRemovalPreviewRowsAll,
      existingModels,
      selectedModelSet,
    ]
  )

  // 厂商分类按 a-z 排序，Other 放最后，便于查找
  const getSortedCategoryEntries = <T,>(
    categories: Record<string, T[]>
  ): [string, T[]][] =>
    Object.entries(categories).sort(([a], [b]) => {
      if (a === 'Other') return 1
      if (b === 'Other') return -1
      return a.localeCompare(b, undefined, { sensitivity: 'base' })
    })

  const toggleModel = (model: string) => {
    setSelectedModels((prev) =>
      prev.includes(model) ? prev.filter((m) => m !== model) : [...prev, model]
    )
  }

  const selectCompactModel = (model: string) => {
    setSelectedCompactModels((prev) => addNormalizedModel(prev, model))
  }

  const deselectCompactModel = (model: string) => {
    const entry = compactEntryMap.get(normalizeModelName(model))
    setSelectedCompactModels((prev) => removeNormalizedModel(prev, model))
    if (entry?.shouldMap) {
      setSelectedExistingModels((prev) =>
        addNormalizedModel(prev, entry.upstreamModel)
      )
    }
  }

  const keepExistingModel = (model: string) => {
    const entry = compactEntryByUpstreamModel.get(normalizeModelName(model))
    setSelectedExistingModels((prev) => addNormalizedModel(prev, model))
    if (entry) {
      setSelectedCompactModels((prev) =>
        removeNormalizedModel(prev, entry.model)
      )
    }
  }

  const removeExistingModel = (model: string) => {
    const entry = compactEntryMap.get(normalizeModelName(model))
    setSelectedExistingModels((prev) => removeNormalizedModel(prev, model))
    if (entry) {
      setSelectedCompactModels((prev) =>
        removeNormalizedModel(prev, entry.model)
      )
    }
  }

  const toggleCompactRow = (row: ModelReconciliationRow) => {
    if (row.id.startsWith('compact-new:')) {
      if (row.checked) {
        deselectCompactModel(row.model)
      } else {
        selectCompactModel(row.model)
      }
      return
    }

    if (row.id.startsWith('removed:')) {
      if (row.checked) {
        removeExistingModel(row.model)
      } else {
        keepExistingModel(row.model)
      }
      return
    }

    removeExistingModel(row.model)
  }

  const toggleCategory = (categoryModels: string[], isChecked: boolean) => {
    setSelectedModels((prev) => {
      if (isChecked) {
        const newSelected = [...prev]
        categoryModels.forEach((model) => {
          if (!newSelected.includes(model)) {
            newSelected.push(model)
          }
        })
        return newSelected
      } else {
        return prev.filter((m) => !categoryModels.includes(m))
      }
    })
  }

  const toggleCompactCategory = (
    categoryRows: ModelReconciliationRow[],
    isChecked: boolean
  ) => {
    categoryRows.forEach((row) => {
      if (isChecked) {
        if (row.id.startsWith('compact-new:')) {
          selectCompactModel(row.model)
        } else {
          keepExistingModel(row.model)
        }
        return
      }

      if (row.id.startsWith('compact-new:')) {
        deselectCompactModel(row.model)
      } else if (row.id.startsWith('existing:')) {
        removeExistingModel(row.model)
      } else if (row.id.startsWith('removed:')) {
        removeExistingModel(row.model)
      }
    })
  }

  const selectAllVisibleModels = () => {
    if (compactMode) {
      toggleCompactCategory(activeCompactRows, true)
      return
    }

    setSelectedModels((prev) => {
      const newSelected = [...prev]
      activeTabModels.forEach((model) => {
        if (!newSelected.includes(model)) {
          newSelected.push(model)
        }
      })
      return newSelected
    })
  }

  const deselectVisibleModels = () => {
    if (compactMode) {
      toggleCompactCategory(activeCompactRows, false)
      return
    }

    setSelectedModels((prev) =>
      prev.filter((model) => !activeTabModels.includes(model))
    )
  }

  const clearSelection = () => {
    if (compactMode) {
      setSelectedCompactModels([])
      setSelectedExistingModels([])
      return
    }

    setSelectedModels([])
  }

  const getEmptyTabMessage = () => {
    if (searchKeyword.trim()) return t('No matching results')
    if (activeTab === 'new') return t('No models to add')
    if (activeTab === 'removed') return t('No models to remove')
    return t('No models found.')
  }

  const activeRegularRows = useMemo<ModelReconciliationRow[]>(
    () =>
      activeTabModels.map((model) => {
        const normalized = normalizeModelName(model)
        const redirectOnly = redirectOnlySet.has(normalized)
        return {
          id: `regular:${model}`,
          model,
          checked: selectedModelSet.has(normalized),
          className: getReconciliationModelClassName({
            existedLocally: existingModelSet.has(normalized),
            selected: selectedModelSet.has(normalized),
          }),
          hint: redirectOnly ? (
            <Info className='text-warning mt-0.5 h-3.5 w-3.5 shrink-0' />
          ) : undefined,
          hintText: redirectOnly
            ? t('From model redirect, not yet added to models list')
            : undefined,
        }
      }),
    [activeTabModels, existingModelSet, redirectOnlySet, selectedModelSet, t]
  )
  const activeReconciliationRows = compactMode
    ? activeCompactRows
    : activeRegularRows
  const activeRowsByCategory = useMemo(() => {
    const categories: Record<string, ModelReconciliationRow[]> = {}
    activeReconciliationRows.forEach((row) => {
      const category = categorizeModels([row.model])
      const categoryName = Object.keys(category)[0] ?? 'Other'
      categories[categoryName] ??= []
      categories[categoryName].push(row)
    })
    return categories
  }, [activeReconciliationRows])

  const renderPreviewList = (models: string[], emptyText: string) => {
    if (models.length === 0) {
      return <p className='text-muted-foreground text-sm'>{emptyText}</p>
    }

    return (
      <div className='max-h-28 space-y-1 overflow-y-auto pr-1'>
        {models.map((model) => (
          <div
            key={model}
            className='bg-background rounded-md border px-2 py-1.5 font-mono text-xs leading-5 [overflow-wrap:anywhere] break-words'
            title={model}
          >
            {model}
          </div>
        ))}
      </div>
    )
  }

  const renderCompactRemovalPreviewList = () => {
    if (compactRemovalPreviewRowsAll.length === 0) {
      return (
        <p className='text-muted-foreground text-sm'>
          {t('No models to remove')}
        </p>
      )
    }

    return (
      <div className='max-h-28 space-y-1 overflow-y-auto pr-1'>
        {compactRemovalPreviewRowsAll.map((row) => (
          <div
            key={row.id}
            className='bg-background rounded-md border px-2 py-1.5 text-xs leading-5'
            title={row.model}
          >
            <div className='flex min-w-0 flex-col items-start gap-1'>
              <span className='min-w-0 flex-1 font-mono [overflow-wrap:anywhere] break-words'>
                {row.model}
              </span>
              <Badge
                variant='outline'
                className={cn('max-w-full', row.badgeClassName)}
                title={row.badgeLabel}
              >
                <span className='min-w-0 truncate'>{row.badgeLabel}</span>
              </Badge>
            </div>
          </div>
        ))}
      </div>
    )
  }

  const handleToggleReconciliationRow = (row: ModelReconciliationRow) => {
    if (compactMode) {
      toggleCompactRow(row)
      return
    }
    toggleModel(row.model)
  }

  const handleToggleReconciliationCategory = (
    rows: ModelReconciliationRow[],
    checked: boolean
  ) => {
    if (compactMode) {
      toggleCompactCategory(rows, checked)
      return
    }
    toggleCategory(
      rows.map((row) => row.model),
      checked
    )
  }

  const showFooterActions =
    !!(activeChannel || customFetcher) &&
    !isFetching &&
    (fetchedModels.length > 0 ||
      removedModels.length > 0 ||
      (compactMode && existingModels.length > 0))
  const dialogTitle = compactMode
    ? t('Fetch Compact Models')
    : t('Fetch Models')
  const dialogDescription = (() => {
    if (activeChannel) {
      return (
        <>
          {t('Channel:')} <strong>{activeChannel.name}</strong>
        </>
      )
    }
    if (channelName) {
      return (
        <>
          {t('Channel:')} <strong>{channelName}</strong>
        </>
      )
    }
    return t('Fetch available models from upstream')
  })()
  const saveLabel = (() => {
    if (isSaving) return t('Saving...')
    if (compactMode) return t('Apply Compact Models')
    return t('Save Models')
  })()
  let bodyContent
  if (!activeChannel && !customFetcher) {
    bodyContent = (
      <div className='text-muted-foreground py-8 text-center'>
        {t('No channel selected')}
      </div>
    )
  } else if (isFetching) {
    bodyContent = (
      <div className='flex items-center justify-center py-12'>
        <Loader2 className='text-muted-foreground h-8 w-8 animate-spin' />
      </div>
    )
  } else if (
    fetchedModels.length === 0 &&
    removedModels.length === 0 &&
    (!compactMode || existingModels.length === 0)
  ) {
    bodyContent = (
      <div className='text-muted-foreground py-8 text-center'>
        <p>{t('No models fetched yet.')}</p>
        <Button
          className='mt-4'
          onClick={() => handleFetchModels()}
          disabled={isFetching}
        >
          {t('Fetch Models')}
        </Button>
      </div>
    )
  } else {
    bodyContent = (
      <div className='space-y-4'>
        {/* Search Bar */}
        <div className='relative'>
          <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
          <Input
            placeholder={t('Search models...')}
            value={searchKeyword}
            onChange={(e) => setSearchKeyword(e.target.value)}
            className='pl-9'
          />
        </div>

        {activeVisibleCount > 0 ? (
          <div className='bg-muted/30 flex flex-col gap-2 rounded-lg border p-2 sm:flex-row sm:items-center sm:justify-between'>
            <span className='text-muted-foreground px-1 text-xs'>
              {selectedActiveVisibleCount} / {activeVisibleCount}{' '}
              {t('selected')}
            </span>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={selectAllVisibleModels}
                disabled={selectedVisibleModelsCount === activeVisibleCount}
              >
                <CheckCheck className='mr-1.5 h-3.5 w-3.5' />
                {t('Select visible')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={deselectVisibleModels}
                disabled={selectedVisibleModelsCount === 0}
              >
                <MinusCircle className='mr-1.5 h-3.5 w-3.5' />
                {t('Deselect visible')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={clearSelection}
                disabled={
                  compactMode
                    ? selectedCompactModels.length === 0 &&
                      selectedExistingModels.length === 0
                    : selectedModels.length === 0
                }
              >
                <X className='mr-1.5 h-3.5 w-3.5' />
                {t('Clear model list')}
              </Button>
            </div>
          </div>
        ) : null}

        {/* Tabs for New vs Existing vs Removed */}
        <Tabs
          key={`${activeChannel?.id ?? 'custom'}-${fetchedModels.length}-${removedModels.length}`}
          value={activeTab}
          onValueChange={(value) => setActiveTab(value as ModelTab)}
        >
          <TabsList className='grid w-full grid-cols-3'>
            <TabsTrigger value='new'>
              {t('New Models ({{count}})', {
                count: compactMode
                  ? compactNewRowsAll.length
                  : allFetchedNewModels.length,
              })}
            </TabsTrigger>
            <TabsTrigger value='existing'>
              {t('Existing Models ({{count}})', {
                count: compactMode
                  ? compactExistingRowsAll.length
                  : allFetchedExistingModels.length,
              })}
            </TabsTrigger>
            <TabsTrigger value='removed'>
              {t('Removed Models ({{count}})', {
                count: compactMode
                  ? compactRemovedRowsAll.length
                  : allRemovedModels.length,
              })}
            </TabsTrigger>
          </TabsList>

          <TabsContent
            value='new'
            className='max-h-96 space-y-2 overflow-y-auto'
          >
            {activeTab === 'new' && activeVisibleCount === 0 ? (
              <p className='text-muted-foreground py-8 text-center text-sm'>
                {getEmptyTabMessage()}
              </p>
            ) : null}
            {activeTab === 'new' ? (
              <ModelReconciliationList
                categories={getSortedCategoryEntries(activeRowsByCategory)}
                selectedLabel={t('selected')}
                onToggleRow={handleToggleReconciliationRow}
                onToggleCategory={handleToggleReconciliationCategory}
              />
            ) : null}
          </TabsContent>

          <TabsContent
            value='existing'
            className='max-h-96 space-y-2 overflow-y-auto'
          >
            {activeTab === 'existing' && activeVisibleCount === 0 ? (
              <p className='text-muted-foreground py-8 text-center text-sm'>
                {getEmptyTabMessage()}
              </p>
            ) : null}
            {activeTab === 'existing' ? (
              <ModelReconciliationList
                categories={getSortedCategoryEntries(activeRowsByCategory)}
                selectedLabel={t('selected')}
                onToggleRow={handleToggleReconciliationRow}
                onToggleCategory={handleToggleReconciliationCategory}
              />
            ) : null}
          </TabsContent>

          <TabsContent
            value='removed'
            className='max-h-96 space-y-2 overflow-y-auto'
          >
            <p className='text-muted-foreground text-xs'>
              {t(
                'These models are still in your selection but were not returned by the upstream listing. Entries that are only model_mapping source aliases are omitted. Toggle to adjust before saving.'
              )}
            </p>
            {activeTab === 'removed' && activeVisibleCount === 0 ? (
              <p className='text-muted-foreground py-8 text-center text-sm'>
                {getEmptyTabMessage()}
              </p>
            ) : null}
            {activeTab === 'removed' ? (
              <ModelReconciliationList
                categories={getSortedCategoryEntries(activeRowsByCategory)}
                selectedLabel={t('selected')}
                onToggleRow={handleToggleReconciliationRow}
                onToggleCategory={handleToggleReconciliationCategory}
              />
            ) : null}
          </TabsContent>
        </Tabs>

        {/* Selection Summary */}
        <div className='bg-muted/50 rounded-lg border p-3 text-sm'>
          {t('{{n}} model(s) selected', {
            n: compactMode ? compactSelectedModelCount : selectedModels.length,
          })}
          <span className='text-muted-foreground ml-2'>
            ({selectedActiveVisibleCount} / {activeVisibleCount} {t('selected')}
            )
          </span>
        </div>

        <div className='rounded-lg border p-3'>
          <div className='mb-3 flex items-center justify-between gap-2'>
            <p className='text-sm font-medium'>{t('Change preview')}</p>
            {modelsToAdd.length === 0 && modelsToRemove.length === 0 ? (
              <span className='text-muted-foreground text-xs'>
                {t('No model changes')}
              </span>
            ) : null}
          </div>
          <div className='grid gap-3 md:grid-cols-2'>
            <div className='border-success/25 bg-success/10 rounded-md border p-3'>
              <p className='text-success mb-2 text-sm font-medium'>
                {t('Models to add')} ({modelsToAdd.length})
              </p>
              {renderPreviewList(modelsToAdd, t('No models to add'))}
            </div>
            <div className='border-destructive/25 bg-destructive/10 rounded-md border p-3'>
              <p className='text-destructive mb-2 text-sm font-medium'>
                {t('Models to remove')} ({modelsToRemove.length})
              </p>
              {compactMode
                ? renderCompactRemovalPreviewList()
                : renderPreviewList(modelsToRemove, t('No models to remove'))}
            </div>
          </div>
        </div>
      </div>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleClose}
      title={dialogTitle}
      description={dialogDescription}
      contentClassName='w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] sm:min-w-[min(44rem,calc(100vw-3rem))] sm:w-[min(max(44rem,92vw),calc(100vw-3rem),96rem)] sm:max-w-[min(max(44rem,92vw),calc(100vw-3rem),96rem)]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        showFooterActions ? (
          <>
            <Button variant='outline' onClick={handleClose} disabled={isSaving}>
              {t('Cancel')}
            </Button>
            <Button onClick={handleSave} disabled={isSaving}>
              {isSaving && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
              {saveLabel}
            </Button>
          </>
        ) : null
      }
    >
      {bodyContent}
    </Dialog>
  )
}
