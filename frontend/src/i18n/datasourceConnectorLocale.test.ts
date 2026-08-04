import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

import { LOCALE_BUNDLES, getLocaleValueAtPath, type LocaleName } from './localeKeyAudit.ts'

const DIALOG_PATH = join(
  dirname(fileURLToPath(import.meta.url)),
  '../views/knowledge/settings/DataSourceEditorDialog.vue',
)

// The connector picker resolves its label and description through computed keys
// (`datasource.connector.${def.type}`), which the static usage audit cannot
// see. A type declared in connectorDefs but absent from a locale therefore
// renders the raw key in the UI while every other check stays green, so assert
// the two bags directly against the connector list that drives the picker.
function declaredConnectorTypes(): string[] {
  const source = readFileSync(DIALOG_PATH, 'utf8')
  const start = source.indexOf('const connectorDefs')
  assert.notEqual(start, -1, 'connectorDefs not found in DataSourceEditorDialog.vue')
  const end = source.indexOf('const currentDef', start)
  assert.notEqual(end, -1, 'end of connectorDefs not found')
  const types = [...source.slice(start, end).matchAll(/^\s*type:\s*'([^']+)'/gm)].map((m) => m[1])
  assert.ok(types.length > 0, 'no connector types parsed from connectorDefs')
  return types
}

test('every connector type has a name and description in every locale', () => {
  const types = declaredConnectorTypes()
  const failures: string[] = []

  for (const type of types) {
    for (const bag of ['connector', 'connectorDesc'] as const) {
      for (const [localeName, bundle] of Object.entries(LOCALE_BUNDLES) as Array<
        [LocaleName, unknown]
      >) {
        const path = `datasource.${bag}.${type}`
        const label = getLocaleValueAtPath(bundle, path)
        if (typeof label !== 'string' || !label) failures.push(`${localeName}: missing ${path}`)
      }
    }
  }

  assert.deepEqual(failures, [], failures.join('\n'))
})
