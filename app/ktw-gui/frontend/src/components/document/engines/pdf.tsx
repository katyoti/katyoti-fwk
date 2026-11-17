// SPDX-License-Identifier: Apache-2.0
// PDF engine — application/pdf, in the web view's own PDF viewer (from a blob
// URL: a data: URL of a real PDF is too long for it).
import { useEffect, useState } from 'react'
import { MEDIA_TYPE_PDF } from '../../../generated/media-types'
import type { DisplayEngine, EngineProps } from '../engines'

function PdfView({ doc }: EngineProps) {
  const [url, setUrl] = useState<string>()
  useEffect(() => {
    if (typeof doc.content !== 'string' || doc.encoding !== 'base64') return
    const bin = atob(doc.content)
    const bytes = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
    const u = URL.createObjectURL(new Blob([bytes], { type: MEDIA_TYPE_PDF }))
    setUrl(u)
    return () => URL.revokeObjectURL(u)
  }, [doc.content, doc.encoding])
  if (!url) return null
  return <iframe src={url} title={doc.title ?? 'PDF'} style={{ width: '100%', height: '100%', border: 'none' }} />
}

export const pdfEngine: DisplayEngine = {
  id: 'pdf',
  label: 'PDF',
  accepts: [MEDIA_TYPE_PDF],
  capabilities: { view: true, save: true },
  builtin: true,
  component: PdfView,
}
