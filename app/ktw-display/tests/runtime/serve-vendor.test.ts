// SPDX-License-Identifier: Apache-2.0
// The vendor route serves the draw.io editor's files by their real names.
// It used to lower-case the whole path, which finds `js/PreConfig.js` on
// Windows only — a case-sensitive filesystem (macOS, Linux) has no
// `js/preconfig.js`. The editor is gitignored (vendor/drawio/setup-editor.*),
// so the file tests skip where it is not fetched.
import { describe, expect, test } from 'bun:test'
import { existsSync, readdirSync } from 'node:fs'
import path from 'node:path'
import { serveVendor } from '../../src/runtime/serve.ts'

const editorJs = path.resolve(import.meta.dir, '../../vendor/drawio/editor/js')
const mixedCase = existsSync(editorJs) ? readdirSync(editorJs).find((f) => /[A-Z]/.test(f)) : undefined

describe('serveVendor', () => {
  test('the embedded viewer is found whatever the case asked', async () => {
    expect((await serveVendor('drawio/viewer-static.min.js')).status).toBe(200)
    expect((await serveVendor('DRAWIO/Viewer-Static.min.js')).status).toBe(200)
  })

  test.skipIf(!mixedCase)('an editor file with capitals is served under its own name', async () => {
    const res = await serveVendor(`drawio/editor/js/${mixedCase}`)
    expect(res.status).toBe(200)
  })

  test('a path climbing out of the vendor tree is refused', async () => {
    expect((await serveVendor('drawio/../../package.json')).status).toBe(403)
  })
})
