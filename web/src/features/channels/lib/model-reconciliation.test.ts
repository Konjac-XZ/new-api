import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getReconciliationModelClassName } from './model-reconciliation'

describe('model reconciliation presentation', () => {
  test('keeps existing models black when they remain selected', () => {
    assert.equal(
      getReconciliationModelClassName({
        existedLocally: true,
        selected: true,
      }),
      ''
    )
  })

  test('marks an existing model red only when it is deselected for removal', () => {
    assert.equal(
      getReconciliationModelClassName({
        existedLocally: true,
        selected: false,
      }),
      'font-medium text-destructive'
    )
  })

  test('marks a new model green only when it is selected for addition', () => {
    assert.equal(
      getReconciliationModelClassName({
        existedLocally: false,
        selected: true,
      }),
      'font-medium text-success'
    )
    assert.equal(
      getReconciliationModelClassName({
        existedLocally: false,
        selected: false,
      }),
      ''
    )
  })
})
