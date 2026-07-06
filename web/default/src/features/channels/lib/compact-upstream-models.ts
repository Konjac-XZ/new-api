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
  formatModelsArray,
  normalizeModelName,
} from './model-mapping-validation'

export type CompactUpstreamModelEntry = {
  model: string
  upstreamModel: string
  shouldMap: boolean
}

export type CompactUpstreamModelResult = {
  success: true
  models: string[]
  modelMapping: string
  compactModels: string[]
  entries: CompactUpstreamModelEntry[]
  duplicateModels: string[]
  conflictModels: string[]
}

export type CompactUpstreamModelError = {
  success: false
  error: string
}

export type CompactUpstreamModelBuildResult =
  | CompactUpstreamModelResult
  | CompactUpstreamModelError

function parseModelMapping(
  modelMapping: string
):
  | { success: true; mapping: Record<string, string> }
  | CompactUpstreamModelError {
  const trimmed = modelMapping.trim()
  if (!trimmed) return { success: true, mapping: {} }

  try {
    const parsed = JSON.parse(trimmed)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {
        success: false,
        error: 'Model mapping must be a valid JSON object',
      }
    }

    const mapping: Record<string, string> = {}
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof value !== 'string') {
        return {
          success: false,
          error: 'Model mapping values must be strings',
        }
      }
      mapping[key] = value
    }
    return { success: true, mapping }
  } catch {
    return {
      success: false,
      error: 'Model mapping must be valid JSON format',
    }
  }
}

export function getCompactModelName(upstreamModel: string): string {
  const trimmed = normalizeModelName(upstreamModel)
  if (!trimmed.includes('/')) return trimmed

  const parts = trimmed
    .split('/')
    .map((part) => part.trim())
    .filter(Boolean)

  return parts.at(-1) || trimmed
}

export function buildCompactUpstreamModels(params: {
  upstreamModels: string[]
  existingModels: string[]
  existingModelMapping: string
  selectedCompactModels?: string[]
  selectedExistingModels?: string[]
}): CompactUpstreamModelBuildResult {
  const parsedMapping = parseModelMapping(params.existingModelMapping)
  if (!parsedMapping.success) return parsedMapping

  const duplicateModels = new Set<string>()
  const conflictModels = new Set<string>()
  const seenCompactModels = new Set<string>()
  const entries: CompactUpstreamModelEntry[] = []

  for (const upstreamModel of params.upstreamModels) {
    const normalizedUpstream = normalizeModelName(upstreamModel)
    if (!normalizedUpstream) continue

    const compactModel = getCompactModelName(normalizedUpstream)
    if (!compactModel) continue

    if (seenCompactModels.has(compactModel)) {
      duplicateModels.add(compactModel)
      continue
    }
    seenCompactModels.add(compactModel)

    const existingTarget = parsedMapping.mapping[compactModel]
    const shouldMap =
      normalizedUpstream.includes('/') && compactModel !== normalizedUpstream

    if (shouldMap && existingTarget && existingTarget !== normalizedUpstream) {
      conflictModels.add(compactModel)
    }

    entries.push({
      model: compactModel,
      upstreamModel: normalizedUpstream,
      shouldMap,
    })
  }

  const selectedSet = new Set(
    (params.selectedCompactModels ?? entries.map((entry) => entry.model))
      .map((model) => normalizeModelName(model))
      .filter(Boolean)
  )
  const selectedExistingSet = new Set(
    (params.selectedExistingModels ?? params.existingModels)
      .map((model) => normalizeModelName(model))
      .filter(Boolean)
  )
  const nextMapping = { ...parsedMapping.mapping }

  for (const entry of entries) {
    if (!selectedSet.has(entry.model)) continue
    if (!entry.shouldMap) continue
    if (Object.hasOwn(nextMapping, entry.model)) {
      continue
    }
    nextMapping[entry.model] = entry.upstreamModel
  }

  const selectedEntries = entries.filter((entry) =>
    selectedSet.has(entry.model)
  )
  const selectedMappedTargets = new Set(
    selectedEntries
      .filter((entry) => entry.shouldMap)
      .map((entry) => entry.upstreamModel)
  )
  const models = formatModelsArray([
    ...params.existingModels.filter(
      (model) =>
        selectedExistingSet.has(normalizeModelName(model)) &&
        !selectedMappedTargets.has(normalizeModelName(model))
    ),
    ...selectedEntries.map((entry) => entry.model),
  ]).split(',')

  const compactModels = entries.map((entry) => entry.model)

  return {
    success: true,
    models: models.filter(Boolean),
    modelMapping:
      Object.keys(nextMapping).length > 0
        ? JSON.stringify(nextMapping, null, 2)
        : '',
    compactModels,
    entries,
    duplicateModels: [...duplicateModels],
    conflictModels: [...conflictModels],
  }
}
