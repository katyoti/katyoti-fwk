// SPDX-License-Identifier: Apache-2.0
/**
 * feedbackErrorText — what the feedback form and its settings show for an
 * error the feedback bindings answer. A known error code is translated in the
 * GUI's language (`unavailable`: the published build has no feedback;
 * `folder-unset` / `folder-unreachable`: the folder setting); any other error
 * is shown as the backend wrote it.
 *
 * `code` is the binding's errorCode; a binding that answers a bare string
 * (FeedbackSaveConfig) passes it as both code and error.
 */

import type { GuideT } from '../../guide/i18n'

export function feedbackErrorText(t: GuideT, code: string | undefined, error: string): string {
  switch (code || error) {
    case 'unavailable':
      return t('feedback.unavailable')
    case 'folder-unset':
      return t('feedback.folderUnset')
    case 'folder-unreachable':
      return t('feedback.folderUnreachable', { error })
  }
  return error
}
