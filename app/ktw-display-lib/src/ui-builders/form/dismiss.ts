// SPDX-License-Identifier: Apache-2.0
// The way out of the surface holding a form.

import { createContext } from 'react'

/** DismissContext closes what holds the nodes under it - a page's modal or
 *  drawer: a form's own « Annuler » there leaves it, running nothing (a form
 *  given its own onCancel keeps it). Undefined anywhere else. */
export const DismissContext = createContext<(() => void) | undefined>(undefined)
