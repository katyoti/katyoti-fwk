// SPDX-License-Identifier: Apache-2.0
import { Component, type ErrorInfo, type ReactNode } from 'react'
import { tFor } from '../i18n'

interface Props  { children: ReactNode }
interface State  { error: Error | null; info: string }

/**
 * Catches React render errors.
 * Fallback uses plain HTML/CSS — no Mantine dependency so it can never fail
 * regardless of where in the tree the error occurred.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null, info: '' }

  static getDerivedStateFromError(error: Error): State {
    return { error, info: '' }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    this.setState({ error, info: info.componentStack ?? '' })
    console.error('[ErrorBoundary]', error, info)
  }

  render() {
    if (!this.state.error) return this.props.children
    const t = tFor('components')

    return (
      <div style={{
        height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
        padding: 32, background: '#1a1b1e', color: '#c1c2c5', fontFamily: 'monospace',
      }}>
        <div style={{ maxWidth: 640, width: '100%' }}>
          <div style={{ color: '#fa5252', fontWeight: 600, marginBottom: 8, fontSize: 14 }}>
            {t('errorBoundary.title')}
          </div>
          <div style={{ marginBottom: 12, fontSize: 13 }}>
            {this.state.error.message}
          </div>
          {this.state.info && (
            <pre style={{
              background: '#141517', borderRadius: 6, padding: '10px 14px',
              fontSize: 10, maxHeight: 280, overflow: 'auto', color: '#868e96',
              whiteSpace: 'pre', margin: '0 0 12px',
            }}>
              {this.state.info}
            </pre>
          )}
          <button
            style={{
              background: 'transparent', border: '1px solid #373a40',
              color: '#c1c2c5', borderRadius: 4, padding: '4px 12px',
              cursor: 'pointer', fontSize: 12,
            }}
            onClick={() => this.setState({ error: null, info: '' })}
          >
            {t('errorBoundary.dismiss')}
          </button>
        </div>
      </div>
    )
  }
}
