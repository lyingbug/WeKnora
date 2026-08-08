import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

import enUS from './en-US.ts'
import koKR from './ko-KR.ts'
import ruRU from './ru-RU.ts'
import zhCN from './zh-CN.ts'

const locales = { 'zh-CN': zhCN, 'en-US': enUS, 'ko-KR': koKR, 'ru-RU': ruRU }

// The memory feature shipped labels that rendered as raw keys, twice: once
// because a key was never added to a locale, once because it was added in a
// shape i18n could not resolve. Neither shows up in a type-check or a build —
// the string simply comes out wrong at runtime, in one language, on one screen.
// So the references are checked against the locale files directly.

const SOURCE_ROOTS = ['src/views/memory', 'src/views/settings', 'src/components']

function sourceFiles(root: string): string[] {
  let entries: string[]
  try {
    entries = readdirSync(root)
  } catch {
    return []
  }
  return entries.flatMap((entry) => {
    const path = join(root, entry)
    if (statSync(path).isDirectory()) return sourceFiles(path)
    return path.endsWith('.vue') || path.endsWith('.ts') ? [path] : []
  })
}

/** Statically referenced memory.* keys, i.e. those written as literals. */
function referencedKeys(): Map<string, string> {
  const found = new Map<string, string>()
  for (const root of SOURCE_ROOTS) {
    for (const file of sourceFiles(root)) {
      const source = readFileSync(file, 'utf8')
      for (const match of source.matchAll(/\$?t\(\s*['"](memory\.[A-Za-z0-9_.]+)['"]/g)) {
        found.set(match[1], file)
      }
    }
  }
  return found
}

function resolve(locale: unknown, key: string): unknown {
  return key.split('.').reduce<any>((node, part) => (node == null ? undefined : node[part]), locale)
}

test('every referenced memory translation key exists in every locale', () => {
  const refs = referencedKeys()
  assert.ok(refs.size > 0, 'found no memory translation references, so this test proves nothing')

  const failures: string[] = []
  for (const [key, file] of refs) {
    for (const [name, locale] of Object.entries(locales)) {
      const value = resolve(locale, key)
      if (typeof value !== 'string' || value.trim() === '') {
        failures.push(`${name}: ${key} (referenced in ${file})`)
      }
    }
  }
  assert.deepEqual(failures, [], `missing or empty translations:\n${failures.join('\n')}`)
})
