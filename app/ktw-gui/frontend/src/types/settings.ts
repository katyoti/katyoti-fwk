// SPDX-License-Identifier: Apache-2.0
// Application settings — typed structure persisted in Zustand (and eventually to disk).

// ── Appearance ────────────────────────────────────────────────────────────────

export type ThemeMode   = 'dark' | 'light' | 'system'
export type FontFamily  = 'Inter' | 'Roboto' | 'Open Sans' | 'Lato' | 'System UI'
export type MonoFamily  = 'JetBrains Mono' | 'Fira Code' | 'Cascadia Code' | 'Source Code Pro' | 'Consolas'
export type CodeTheme   = 'github-dark' | 'github-light' | 'dracula' | 'one-dark' | 'nord'

export interface AppearanceSettings {
  themeMode:    ThemeMode
  accentColor:  string       // hex e.g. '#228be6'
  fontSize:     number       // px, 10–20
  fontFamily:   FontFamily
  monoFamily:   MonoFamily
  codeThemeDark:  CodeTheme
  codeThemeLight: CodeTheme
}

// ── General ───────────────────────────────────────────────────────────────────

/** The person's display level, once for all universes: Essentiel
 *  a newcomer's surfaces, Standard the daily ones, Expert everything. */
export type DisplayLevel = 'essential' | 'standard' | 'expert'
export const DISPLAY_LEVELS: DisplayLevel[] = ['essential', 'standard', 'expert']

export interface GeneralSettings {
  level:             DisplayLevel
  autoScroll:        boolean
  showToolInputs:    boolean
  compactMessages:   boolean
  fullWidthMessages: boolean   // true = stretch to panel width; false = natural/content width
}

// ── Universes ─────────────────────────────────────────────────────────────────
//
// Universe-management preferences. preferredDefault is the universe to show
// on first launch when no per-window localStorage is set. Per-window
// last-displayed (ktw.lastDisplayed) wins over this. Empty = falls back to
// the admin universe.

export interface UniverseSettings {
  preferredDefault: string
  /** The tasks the person starred, per universe (task URIs): Home's quick actions. */
  starred?: Record<string, string[]>
  /** The order picked per list (tasks, skills): name, category or source. */
  sort?: Record<string, string>
  /** The quick actions the person removed, per universe (a built-in's id or
   *  task:<uri>): Home leaves them out until starred again. */
  removed?: Record<string, string[]>
}

// ── Notifications ─────────────────────────────────────────────────────────────

export interface NotificationSettings {
  // reserved — not active yet
  enabled: boolean
}

// ── Tree ──────────────────────────────────────────────────────────────────────
//
// How the projects tree draws its rows, chosen in the tree's
// settings menu. Each is off by default, so the tree shows what it always did.

export interface TreeSettings {
  /** The type badge left out of the rows (the type's icon stays). */
  hideTypes: boolean
}

// ── Root ──────────────────────────────────────────────────────────────────────

export interface AppSettings {
  appearance:    AppearanceSettings
  general:       GeneralSettings
  universes:     UniverseSettings
  notifications: NotificationSettings
  tree:          TreeSettings
}

export const DEFAULT_SETTINGS: AppSettings = {
  appearance: {
    themeMode:      'dark',
    accentColor:    '#228be6',
    fontSize:       14,
    fontFamily:     'Inter',
    monoFamily:     'JetBrains Mono',
    codeThemeDark:  'github-dark',
    codeThemeLight: 'github-light',
  },
  general: {
    level:             'essential',
    autoScroll:        true,
    showToolInputs:    true,
    compactMessages:   false,
    fullWidthMessages: false,
  },
  universes: {
    preferredDefault: '',
  },
  notifications: {
    enabled: false,
  },
  tree: {
    hideTypes: false,
  },
}

/**
 * The settings read at start (GuiSettings.yaml) over the defaults, section by
 * section, so a missing field keeps its default. Every section is merged —
 * the universes one too: without it the first save wrote the defaults over the
 * preferred universe, the starred tasks and the sort orders.
 */
export function mergeLoadedSettings(loaded: Partial<AppSettings> | null | undefined): AppSettings {
  const l = loaded ?? {}
  return {
    appearance:    { ...DEFAULT_SETTINGS.appearance,    ...(l.appearance    ?? {}) },
    general:       { ...DEFAULT_SETTINGS.general,       ...(l.general       ?? {}) },
    universes:     { ...DEFAULT_SETTINGS.universes,     ...(l.universes     ?? {}) },
    notifications: { ...DEFAULT_SETTINGS.notifications, ...(l.notifications ?? {}) },
    tree:          { ...DEFAULT_SETTINGS.tree,          ...(l.tree          ?? {}) },
  }
}

export type SettingsSection = 'general' | 'appearance' | 'universes' | 'notifications' | 'feedback'
