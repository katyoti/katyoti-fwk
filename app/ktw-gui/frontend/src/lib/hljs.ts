// SPDX-License-Identifier: Apache-2.0
// highlight.js with only the languages the text / code engine shows
// (components/document/content.ts `codeLanguage`). vite.config.ts aliases the
// bare `highlight.js` import to this module, so @mantine/code-highlight uses it
// too instead of bundling every language (~1 MB). A language added to
// `codeLanguage` is registered here.
import hljs from 'highlight.js/lib/core'
import bash from 'highlight.js/lib/languages/bash'
import csharp from 'highlight.js/lib/languages/csharp'
import css from 'highlight.js/lib/languages/css'
import diff from 'highlight.js/lib/languages/diff'
import dockerfile from 'highlight.js/lib/languages/dockerfile'
import go from 'highlight.js/lib/languages/go'
import ini from 'highlight.js/lib/languages/ini'
import java from 'highlight.js/lib/languages/java'
import javascript from 'highlight.js/lib/languages/javascript'
import json from 'highlight.js/lib/languages/json'
import markdown from 'highlight.js/lib/languages/markdown'
import plaintext from 'highlight.js/lib/languages/plaintext'
import powershell from 'highlight.js/lib/languages/powershell'
import python from 'highlight.js/lib/languages/python'
import rust from 'highlight.js/lib/languages/rust'
import shell from 'highlight.js/lib/languages/shell'
import sql from 'highlight.js/lib/languages/sql'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import yaml from 'highlight.js/lib/languages/yaml'

const languages = {
  bash, csharp, css, diff, dockerfile, go, ini, java, javascript, json, markdown,
  plaintext, powershell, python, rust, shell, sql, typescript, xml, yaml,
}
for (const [name, lang] of Object.entries(languages)) hljs.registerLanguage(name, lang)

export default hljs
