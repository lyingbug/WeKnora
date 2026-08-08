import assert from 'node:assert/strict'
import test from 'node:test'

import enUS from './en-US.ts'
import koKR from './ko-KR.ts'
import ruRU from './ru-RU.ts'
import zhCN from './zh-CN.ts'

const locales = { 'zh-CN': zhCN, 'en-US': enUS, 'ko-KR': koKR, 'ru-RU': ruRU }

function settingKeyEntries(locale: unknown): Record<string, unknown> {
  const memory = (locale as Record<string, any>)?.memory
  const keys = memory?.settings?.keys
  assert.ok(keys && typeof keys === 'object', 'memory.settings.keys is missing')
  return keys as Record<string, unknown>
}

// Every label in the memory settings panel rendered as its raw key until this
// was found: i18n reads a dot as a path separator, so an entry literally named
// "memory.write.mode" is looked up as memory → write → mode and never resolves.
// The setting keys the backend publishes do contain dots, so the translations
// use dot-free names and the component substitutes underscores.
test('memory setting translation keys are reachable by lookup', () => {
  for (const [name, locale] of Object.entries(locales)) {
    for (const key of Object.keys(settingKeyEntries(locale))) {
      assert.ok(
        !key.includes('.'),
        `${name}: memory.settings.keys.${key} contains a dot and can never be resolved; ` +
          `use ${key.replace(/\./g, '_')} instead`,
      )
    }
  }
})

test('every memory setting translation carries a label', () => {
  for (const [name, locale] of Object.entries(locales)) {
    for (const [key, entry] of Object.entries(settingKeyEntries(locale))) {
      const label = (entry as Record<string, unknown>)?.label
      assert.equal(
        typeof label,
        'string',
        `${name}: memory.settings.keys.${key} has no label`,
      )
      assert.ok(
        (label as string).trim().length > 0,
        `${name}: memory.settings.keys.${key} has an empty label`,
      )
    }
  }
})

// The four locales are edited by hand and drift silently otherwise, which shows
// up as a control that is labelled in one language and keyed in another.
test('the locales describe the same memory settings', () => {
  const reference = Object.keys(settingKeyEntries(zhCN)).sort()
  for (const [name, locale] of Object.entries(locales)) {
    assert.deepEqual(
      Object.keys(settingKeyEntries(locale)).sort(),
      reference,
      `${name} does not cover the same memory settings as zh-CN`,
    )
  }
})

// The keys subtree was guarded and the option labels were not, so the relation
// and extraction-source multi-selects rendered raw tokens (asked_about,
// user_message) in every language. SettingRow falls back to the token when a
// label is absent, which is silent.
//
// The values below are the ones the backend descriptors declare as selectable.
// A hardcoded list drifts from the backend, which is the failure mode this whole
// file exists to catch, so the parity test after it is the real guard: it fails
// when any locale gains or loses an option the others do not have, whatever the
// option happens to be.
const SELECTABLE_OPTIONS = [
  // memory.write.mode
  'off', 'explicit_only', 'gated_auto', 'always_auto',
  // memory.privacy.pii_redaction
  'redact', 'block',
  // memory.embed_visitor_space
  'session_only', 'persistent',
  // anchor relations: decay_exempt_relations and relation_weights
  'mentioned', 'asked_about', 'bookmarked', 'disagreed', 'learned', 'corrected', 'owns',
  // memory.security.extract_sources
  'user_message',
  // memory types: write.allowed_types and recall.always_include_types
  'profile', 'preference', 'project', 'entity', 'topic', 'episode', 'open_question',
  // memory.channels
  'web', 'api', 'im', 'embed',
]

function optionsOf(locale: unknown): Record<string, unknown> {
  return ((locale as Record<string, any>)?.memory?.settings?.options ?? {}) as Record<string, unknown>
}

test('every selectable option has a label in every locale', () => {
  const failures: string[] = []
  for (const [name, locale] of Object.entries(locales)) {
    const options = optionsOf(locale)
    for (const option of SELECTABLE_OPTIONS) {
      const label = options[option]
      if (typeof label !== 'string' || label.trim() === '') {
        failures.push(`${name}: memory.settings.options.${option}`)
      }
    }
  }
  assert.deepEqual(failures, [], `options rendering as raw tokens:\n${failures.join('\n')}`)
})

test('the locales offer the same option labels', () => {
  const reference = Object.keys(optionsOf(zhCN)).sort()
  for (const [name, locale] of Object.entries(locales)) {
    assert.deepEqual(
      Object.keys(optionsOf(locale)).sort(),
      reference,
      `${name} does not label the same options as zh-CN`,
    )
  }
})

test('every setting group has a title and a description in every locale', () => {
  const groups = Object.keys(
    ((zhCN as Record<string, any>).memory?.settings?.groups ?? {}) as Record<string, unknown>,
  )
  assert.ok(groups.length > 0, 'found no setting groups, so this test proves nothing')
  const failures: string[] = []
  for (const [name, locale] of Object.entries(locales)) {
    const declared = (locale as Record<string, any>)?.memory?.settings?.groups ?? {}
    for (const group of groups) {
      for (const field of ['title', 'description']) {
        const value = declared[group]?.[field]
        if (typeof value !== 'string' || value.trim() === '') {
          failures.push(`${name}: memory.settings.groups.${group}.${field}`)
        }
      }
    }
  }
  assert.deepEqual(failures, [], `missing group text:\n${failures.join('\n')}`)
})
