import assert from 'node:assert/strict'
import test from 'node:test'

import {
  findField,
  supportsThinking,
  thinkingControlOf,
  THINKING_CONTROL_NONE,
  type ModelCapabilities,
} from './modelCapabilities.ts'

// The provider table that used to live here is gone: it duplicated the Go
// provider adapters, had to be kept aligned by hand, and drifted. The backend
// now serves the same facts it acts on, so what is left to test is that this
// module reads the manifest correctly rather than that it predicts the backend.

function manifest(overrides: Partial<ModelCapabilities> = {}): ModelCapabilities {
  return {
    vendor: 'aliyun',
    protocol: 'openai-chat',
    supports_thinking: true,
    groups: [{
      key: 'thinking',
      fields: [{
        id: 'thinking.mode',
        kind: 'enum',
        widget: 'select',
        wire_field: 'enable_thinking',
        options: [{ value: 'on' }, { value: 'off' }],
      }],
    }],
    ...overrides,
  }
}

test('thinkingControlOf reports the wire field the backend resolved', () => {
  assert.equal(thinkingControlOf(manifest()), 'enable_thinking')
})

test('a model without a thinking control reports none', () => {
  const withoutThinking = manifest({ supports_thinking: false, groups: [] })
  assert.equal(thinkingControlOf(withoutThinking), THINKING_CONTROL_NONE)
})

test('a missing manifest reports none rather than throwing', () => {
  assert.equal(thinkingControlOf(null), THINKING_CONTROL_NONE)
})

test('findField locates a field across groups', () => {
  const twoGroups = manifest({
    groups: [
      { key: 'sampling', fields: [{ id: 'temperature', kind: 'float', widget: 'slider' }] },
      ...manifest().groups,
    ],
  })
  assert.equal(findField(twoGroups, 'thinking.mode')?.wire_field, 'enable_thinking')
  assert.equal(findField(twoGroups, 'temperature')?.widget, 'slider')
  assert.equal(findField(twoGroups, 'nope'), undefined)
})

test('a stored legacy override still decides, because the backend honors it', () => {
  assert.equal(supportsThinking(manifest({ supports_thinking: false }), 'thinking_type'), true)
  assert.equal(supportsThinking(manifest(), 'none'), false)
  assert.equal(supportsThinking(manifest()), true)
})
