// SPDX-License-Identifier: Apache-2.0
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))

// tsc reads tsconfig paths; Vite needs them mirrored here. Two aliases:
//   - @display/*       — legacy postmessage + bun-side shared TS
//   - @ktw/display-lib — schema-driven UI library extracted 2026-06-02
//                        (sibling package modeled on @ktw/ktsp-sdk)
const DISPLAY_LIB = resolve(__dirname, '../../ktw-display-lib')

export default defineConfig({
  plugins: [react()],
  resolve: {
    // Dedupe: force these packages to resolve to a SINGLE copy across
    // ktw-gui frontend AND @ktw/display-lib. Without this, Vite picks
    // ktw-gui's own copy for ktw-gui code and the workspace .bun/ copy
    // for lib code — two instances of MantineProvider's React context,
    // two instances of React's dispatcher → "MantineProvider was not
    // found in tree" the moment a lib component mounts inside ktw-gui's
    // tree. Same fix shape as the React dedupe we did for ktw-display's
    // bun build.
    dedupe: [
      'react',
      'react-dom',
      'react/jsx-runtime',
      'react-hook-form',
      'react-i18next',
      'react-markdown',
      '@mantine/core',
      '@mantine/charts',
      '@mantine/dates',
      '@mantine/hooks',
      '@tabler/icons-react',
    ],
    alias: [
      // highlight.js trimmed to the languages the text engine shows — the bare
      // import only (`highlight.js/lib/…` still resolves to the package).
      { find: /^highlight\.js$/, replacement: resolve(__dirname, 'src/lib/hljs.ts') },

      // Legacy alias — kept for postmessage.ts + future shared bun-side TS.
      { find: '@display', replacement: resolve(__dirname, '../../ktw-display/src') },

      // Library aliases. Order matters: more specific patterns first so
      // they win against `@ktw/display-lib` bare.
      { find: /^@ktw\/display-lib\/ui-core\/(.*)$/,     replacement: `${DISPLAY_LIB}/src/ui-core/$1` },
      { find: /^@ktw\/display-lib\/ui-builders\/(.*)$/, replacement: `${DISPLAY_LIB}/src/ui-builders/$1` },
      { find: /^@ktw\/display-lib\/utils\/(.*)$/,       replacement: `${DISPLAY_LIB}/src/utils/$1` },
      { find: '@ktw/display-lib/schema',         replacement: `${DISPLAY_LIB}/src/schema/index.ts` },
      { find: '@ktw/display-lib/types',          replacement: `${DISPLAY_LIB}/src/types/index.ts` },
      { find: '@ktw/display-lib/registry-wire',  replacement: `${DISPLAY_LIB}/src/registry-wire.ts` },
      { find: '@ktw/display-lib',                       replacement: `${DISPLAY_LIB}/src/index.ts` },
    ],
  },
})
