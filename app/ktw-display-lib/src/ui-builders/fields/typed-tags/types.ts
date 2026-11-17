// SPDX-License-Identifier: Apache-2.0
// TypedTagsContent - `content: {type: 'typed-tags', lookup, types, free,
// object}`: an array of tags, each a label with its type and the key its
// lookup gave, found through the `lookup` method, free text allowed.

import type { ContentType } from '../../../schema/schema-contents.ts'

export interface TypedTagsContent extends ContentType {
  type: 'typed-tags'
  /** The method finding tags: `{query, object?}` -> `{items: TypedTag[]}`. */
  lookup?: string
  /** The types a tag may have. */
  types?: TagType[]
  /** Free text allowed as a tag with no type. Default: true. */
  free?: boolean
  /** The object the lookup is for. */
  object?: string
}

/** A type a tag may have: its name, label and colour. */
export type TagType = {
  name: string
  label?: string
  color?: string
}

/** A tag: its label, its type and key when a lookup found it. */
export interface TypedTag {
  label: string
  type?: string
  key?: string
}
