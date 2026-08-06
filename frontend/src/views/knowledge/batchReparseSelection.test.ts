import assert from 'node:assert/strict'
import test from 'node:test'

import { canSelectAllFiltered, hasActiveDocumentFilter } from './batchReparseSelection.ts'

test('a status filter counts as an active document filter', () => {
  assert.equal(hasActiveDocumentFilter({ parse_status: 'failed' }), true)
  assert.equal(hasActiveDocumentFilter({ keyword: 'report' }), true)
  assert.equal(hasActiveDocumentFilter({ folder_path: '/2026' }), true)
})

test('browsing the knowledge base root is not a filter', () => {
  assert.equal(hasActiveDocumentFilter({ folder_path: '', folder_recursive: true }), false)
  assert.equal(hasActiveDocumentFilter({}), false)
  assert.equal(hasActiveDocumentFilter(undefined), false)
})

test('select-all-matches is offered only when the filter reaches past the ticked rows', () => {
  assert.equal(
    canSelectAllFiltered({
      filterActive: true,
      allFilteredSelected: false,
      filteredTotal: 100,
      selectedCount: 35,
    }),
    true,
  )
  assert.equal(
    canSelectAllFiltered({
      filterActive: true,
      allFilteredSelected: false,
      filteredTotal: 35,
      selectedCount: 35,
    }),
    false,
  )
  assert.equal(
    canSelectAllFiltered({
      filterActive: false,
      allFilteredSelected: false,
      filteredTotal: 100,
      selectedCount: 0,
    }),
    false,
  )
  assert.equal(
    canSelectAllFiltered({
      filterActive: true,
      allFilteredSelected: true,
      filteredTotal: 100,
      selectedCount: 35,
    }),
    false,
  )
})
