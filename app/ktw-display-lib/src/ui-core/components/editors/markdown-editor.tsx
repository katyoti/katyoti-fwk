// SPDX-License-Identifier: Apache-2.0
// Lightweight markdown editor — Mantine Textarea + react-markdown preview.
// Replaces the @core/cpm-core/editors MarkdownEditor that the legacy
// renderers imported. v1 is split-view (write left, preview right);
// v2 (gui-rich-text) can replace this with a richer toolbar / WYSIWYG.

import { Tabs, Textarea } from '@mantine/core'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

export interface MarkdownEditorProps {
  value: string
  onChange: (value: string) => void
  label?: React.ReactNode
  description?: React.ReactNode
  placeholder?: string
  required?: boolean
  disabled?: boolean
  readOnly?: boolean
  error?: React.ReactNode
  /** Pixel height of the editor pane. */
  height?: number
  /** data-testid placed on the actual <textarea> input so test automation
   *  can `fill` it directly. Optional. */
  testId?: string
}

export function MarkdownEditor(props: MarkdownEditorProps): React.ReactElement {
  const {
    value,
    onChange,
    label,
    description,
    placeholder,
    required,
    disabled,
    readOnly,
    error,
    height = 300,
    testId,
  } = props

  return (
    <Tabs defaultValue="write" variant="outline">
      {label && (
        <div style={{ marginBottom: 4, fontSize: 14, fontWeight: 500 }}>
          {label}
          {required ? ' *' : ''}
        </div>
      )}
      {description && (
        <div style={{ marginBottom: 6, fontSize: 12, color: 'var(--mantine-color-dimmed)' }}>
          {description}
        </div>
      )}
      <Tabs.List>
        <Tabs.Tab value="write">Write</Tabs.Tab>
        <Tabs.Tab value="preview">Preview</Tabs.Tab>
      </Tabs.List>
      <Tabs.Panel value="write" pt="xs">
        <Textarea
          value={value}
          onChange={(e) => onChange(e.currentTarget.value)}
          placeholder={placeholder}
          disabled={disabled}
          readOnly={readOnly}
          error={error}
          rows={Math.max(4, Math.floor(height / 24))}
          styles={{ input: { fontFamily: 'ui-monospace, SFMono-Regular, monospace', resize: 'vertical' } }}
          data-testid={testId}
        />
      </Tabs.Panel>
      <Tabs.Panel value="preview" pt="xs">
        <div
          style={{
            border: '1px solid var(--mantine-color-default-border)',
            borderRadius: 4,
            padding: 12,
            minHeight: height,
            background: 'var(--mantine-color-default)',
          }}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{value || '_Nothing to preview._'}</ReactMarkdown>
        </div>
      </Tabs.Panel>
    </Tabs>
  )
}
