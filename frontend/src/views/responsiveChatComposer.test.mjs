import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (relative) => readFileSync(new URL(relative, import.meta.url), 'utf8')

const inputField = read('../components/Input-field.vue')
const createChat = read('./creatChat/creatChat.vue')
const knowledgeBase = read('./knowledge/KnowledgeBase.vue')
const platform = read('./platform/index.vue')
const settings = read('./settings/Settings.vue')

// The composer used to be sized by its host pages at four hard-coded
// breakpoints, which left a dead gap between the typing area and the box
// border between 768px and 1044px (issue #917).
for (const [name, source] of [['creatChat', createChat], ['KnowledgeBase', knowledgeBase]]) {
  test(`${name} does not pin the composer textarea to a pixel width`, () => {
    assert.doesNotMatch(source, /t-textarea__inner\)\s*\{[^}]*width:\s*\d+px\s*!important/)
  })

  test(`${name} does not offset the composer with a hard-coded translateX`, () => {
    assert.doesNotMatch(source, /\.answers-input\s*\{[^}]*translateX\(-\d+px\)/)
  })
}

test('composer sizes itself from its container and keeps narrow-screen gutters', () => {
  assert.match(inputField, /\.answers-input\s*\{[^}]*width:\s*100%;[^}]*min-width:\s*0;[^}]*box-sizing:\s*border-box;/)
  assert.match(inputField, /\.not-desktop\(\{\s*padding-inline:\s*16px;/)
  assert.match(inputField, /\.mobile\(\{\s*padding-inline:\s*12px;/)
})

test('the platform shell can shrink below the former desktop minimum width', () => {
  assert.doesNotMatch(platform, /\.main\s*\{[^}]*min-width:\s*600px/)
})

test('the control bar stays on one row on phones', () => {
  // flex-wrap would push the model selector and send button onto a second
  // row, growing the composer and moving send out of thumb reach.
  assert.match(inputField, /\.mobile\(\{[\s\S]*?\.control-bar\s*\{[^}]*flex-wrap:\s*nowrap;/)
  assert.match(inputField, /\.mobile\(\{[\s\S]*?\.control-left\s*\{[^}]*overflow-x:\s*auto;/)
})

test('settings rows stack their label above the control on phones', () => {
  // The setting pages pin the control column to min-width: 280px and give
  // the select an inline width, which crushes the label column to one
  // character per line at 360px.
  const phoneBlock = /@media screen and \(max-width: 767px\) \{[\s\S]*?\n\}/.exec(settings)?.[0] ?? ''
  assert.match(phoneBlock, /:deep\(\.settings-content \.setting-row\)\s*\{[^}]*flex-direction:\s*column;/)
  assert.match(phoneBlock, /\.setting-row > \.setting-control\)\s*\{[^}]*min-width:\s*0;/)
  assert.match(phoneBlock, /\.setting-row > \.setting-control > \*\)\s*\{[^}]*width:\s*100%\s*!important;/)
})

test('the tablet sidebar collapse watcher runs on first render', () => {
  // Breakpoints are resolved at module load, so a non-immediate watcher
  // never fires when the app opens straight into a tablet width.
  assert.match(platform, /watch\(isTabletRef,[\s\S]*?\{\s*immediate:\s*true\s*\}\)/)
})

test('image drag suppression stays in a global style block', () => {
  // `img { user-drag: none }` applies app-wide; a scoped block would only
  // reach this component's own template.
  const globalBlocks = platform.match(/<style lang="less">[\s\S]*?<\/style>/g) ?? []
  assert.ok(
    globalBlocks.some((block) => /\bimg\s*\{[^}]*user-drag:\s*none/.test(block)),
    'expected the img user-drag rule to live in an unscoped <style> block',
  )
})
