import assert from 'node:assert/strict'
import test from 'node:test'

import {
  MOBILE_MAX,
  TABLET_MAX,
  evalBreakpoint,
  isDesktop,
  isMobile,
  isTablet,
  resolveBreakpoint,
} from './useBreakpoint.ts'

type FakeWindow = {
  innerWidth: number
  matchMedia?: (query: string) => { matches: boolean }
}

function withWindow(fake: FakeWindow, run: () => void) {
  const globalWithWindow = globalThis as { window?: unknown }
  const had = 'window' in globalWithWindow
  const previous = globalWithWindow.window
  globalWithWindow.window = fake
  try {
    run()
  } finally {
    if (had) globalWithWindow.window = previous
    else delete globalWithWindow.window
  }
}

/** matchMedia stub whose media width may differ from innerWidth. */
function mediaWindow(innerWidth: number, mediaWidth = innerWidth): FakeWindow {
  return {
    innerWidth,
    matchMedia: (query: string) => {
      const max = /max-width:\s*(\d+)px/.exec(query)
      const min = /min-width:\s*(\d+)px/.exec(query)
      const withinMax = max ? mediaWidth <= Number(max[1]) : true
      const withinMin = min ? mediaWidth >= Number(min[1]) : true
      return { matches: withinMax && withinMin }
    },
  }
}

test('classifies every width into exactly one form factor', () => {
  for (const width of [320, 360, 390, 767, 768, 900, 1023, 1024, 1440, 2560]) {
    const flags = resolveBreakpoint(width)
    const hits = [flags.isMobile, flags.isTablet, flags.isDesktop].filter(Boolean)
    assert.equal(hits.length, 1, `width=${width} matched ${hits.length} form factors`)
  }
})

test('breakpoint edges line up with the CSS media queries', () => {
  assert.deepEqual(resolveBreakpoint(MOBILE_MAX), { isMobile: true, isTablet: false, isDesktop: false })
  assert.deepEqual(resolveBreakpoint(MOBILE_MAX + 1), { isMobile: false, isTablet: true, isDesktop: false })
  assert.deepEqual(resolveBreakpoint(TABLET_MAX), { isMobile: false, isTablet: true, isDesktop: false })
  assert.deepEqual(resolveBreakpoint(TABLET_MAX + 1), { isMobile: false, isTablet: false, isDesktop: true })
})

test('prefers matchMedia so the JS form factor matches the rendered layout', () => {
  // A classic scrollbar makes innerWidth 15px wider than the media-query
  // width: reading innerWidth alone reports desktop while CSS already
  // renders the tablet layout.
  withWindow(mediaWindow(1024, 1009), () => {
    evalBreakpoint()
    assert.equal(isTablet.value, true)
    assert.equal(isDesktop.value, false)
  })
})

test('falls back to the viewport width when matchMedia is unavailable', () => {
  withWindow({ innerWidth: 390 }, () => {
    evalBreakpoint()
    assert.equal(isMobile.value, true)
    assert.equal(isDesktop.value, false)
  })
})

test('defaults to desktop outside a browser so SSR does not throw', () => {
  const globalWithWindow = globalThis as { window?: unknown }
  const had = 'window' in globalWithWindow
  const previous = globalWithWindow.window
  delete globalWithWindow.window
  try {
    evalBreakpoint()
    assert.equal(isDesktop.value, true)
    assert.equal(isMobile.value, false)
    assert.equal(isTablet.value, false)
  } finally {
    if (had) globalWithWindow.window = previous
  }
})
