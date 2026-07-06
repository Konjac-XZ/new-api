import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  buildCompactUpstreamModels,
  getCompactModelName,
} from './compact-upstream-models'

describe('compact upstream model helpers', () => {
  test('uses the final slash segment as the exposed model', () => {
    assert.equal(getCompactModelName('openai/gpt-4o'), 'gpt-4o')
    assert.equal(getCompactModelName('vendor/team/model'), 'model')
  })

  test('adds slashless models without self mappings', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['gpt-4o'],
      existingModels: [],
      existingModelMapping: '',
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.models, ['gpt-4o'])
    assert.equal(result.modelMapping, '')
  })

  test('creates redirects for compacted upstream model names', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o'],
      existingModels: [],
      existingModelMapping: '',
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.models, ['gpt-4o'])
    assert.deepEqual(JSON.parse(result.modelMapping), {
      'gpt-4o': 'openai/gpt-4o',
    })
  })

  test('keeps the first upstream model when compact names collide', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o', 'azure/gpt-4o'],
      existingModels: [],
      existingModelMapping: '',
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.models, ['gpt-4o'])
    assert.deepEqual(result.duplicateModels, ['gpt-4o'])
    assert.deepEqual(JSON.parse(result.modelMapping), {
      'gpt-4o': 'openai/gpt-4o',
    })
  })

  test('does not duplicate existing models', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o', 'anthropic/claude-3-5-sonnet'],
      existingModels: ['gpt-4o'],
      existingModelMapping: '',
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.models, ['gpt-4o', 'claude-3-5-sonnet'])
  })

  test('removes selected mapped upstream targets from exposed models', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o'],
      existingModels: ['openai/gpt-4o'],
      existingModelMapping: '',
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.models, ['gpt-4o'])
    assert.deepEqual(JSON.parse(result.modelMapping), {
      'gpt-4o': 'openai/gpt-4o',
    })
  })

  test('keeps unrelated existing models while compacting mapped upstream targets', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o'],
      existingModels: ['openai/gpt-4o', 'immersive-translate'],
      existingModelMapping: '',
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.models, ['immersive-translate', 'gpt-4o'])
    assert.deepEqual(JSON.parse(result.modelMapping), {
      'gpt-4o': 'openai/gpt-4o',
    })
  })

  test('keeps original upstream target when compact model is not selected', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o'],
      existingModels: ['openai/gpt-4o', 'immersive-translate'],
      existingModelMapping: '',
      selectedCompactModels: [],
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.models, ['openai/gpt-4o', 'immersive-translate'])
    assert.equal(result.modelMapping, '')
  })

  test('removes unrelated existing models only when they are not selected', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o'],
      existingModels: ['openai/gpt-4o', 'immersive-translate'],
      existingModelMapping: '',
      selectedExistingModels: ['openai/gpt-4o'],
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.models, ['gpt-4o'])
    assert.deepEqual(JSON.parse(result.modelMapping), {
      'gpt-4o': 'openai/gpt-4o',
    })
  })

  test('keeps existing redirects when they already point to the same upstream', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o'],
      existingModels: ['gpt-4o'],
      existingModelMapping: JSON.stringify({ 'gpt-4o': 'openai/gpt-4o' }),
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.conflictModels, [])
    assert.deepEqual(JSON.parse(result.modelMapping), {
      'gpt-4o': 'openai/gpt-4o',
    })
  })

  test('preserves conflicting existing redirects', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o'],
      existingModels: ['gpt-4o'],
      existingModelMapping: JSON.stringify({ 'gpt-4o': 'azure/gpt-4o' }),
    })

    assert.equal(result.success, true)
    if (!result.success) return
    assert.deepEqual(result.conflictModels, ['gpt-4o'])
    assert.deepEqual(JSON.parse(result.modelMapping), {
      'gpt-4o': 'azure/gpt-4o',
    })
  })

  test('rejects invalid existing model mapping', () => {
    const result = buildCompactUpstreamModels({
      upstreamModels: ['openai/gpt-4o'],
      existingModels: [],
      existingModelMapping: '{',
    })

    assert.equal(result.success, false)
    if (result.success) return
    assert.equal(result.error, 'Model mapping must be valid JSON format')
  })
})
