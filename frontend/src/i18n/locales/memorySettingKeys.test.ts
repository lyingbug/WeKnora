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
