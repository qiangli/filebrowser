// Per-device File Browser UI preferences.
//
// The embed is single-user and STATELESS server-side (no database), so UI
// preferences are kept in the browser's localStorage instead of the user
// record. They are therefore per-device/per-browser: a phone and a laptop can
// differ, and clearing site data resets them. Keys are namespaced to avoid
// colliding with other apps served from the same origin.

const NS = "fb:";

// The IUser fields persisted client-side. hideDotfiles is also enforced
// server-side, so the listing fetch echoes it back as a ?dotfiles= query param
// (see api/files.ts); the rest are read straight off the overlaid user object.
export const PREF_KEYS = [
  "locale",
  "viewMode",
  "singleClick",
  "redirectAfterCopyMove",
  "dateFormat",
  "aceEditorTheme",
  "hideDotfiles",
  "sorting",
] as const;

export type PrefKey = (typeof PREF_KEYS)[number];

// loadPrefs returns the subset of preferences present in localStorage, parsed
// back to their JSON types. Missing keys are omitted so server-provided code
// defaults show through.
export function loadPrefs(): Partial<IUser> {
  const out: Record<string, unknown> = {};
  for (const key of PREF_KEYS) {
    const raw = localStorage.getItem(NS + key);
    if (raw === null) continue;
    try {
      out[key] = JSON.parse(raw);
    } catch {
      out[key] = raw;
    }
  }
  return out as Partial<IUser>;
}

// savePref persists a single preference to localStorage.
export function savePref(key: PrefKey, value: unknown): void {
  localStorage.setItem(NS + key, JSON.stringify(value));
}
