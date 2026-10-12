/** A page action bound to a key; shown in the soft-key footer and in the help overlay. */
export interface ContextKey {
  /** Single key: a letter, a digit, "/" or "?". Letters match the physical key, so any layout works. */
  key: string;
  /** Short label of the footer keycap, e.g. "поиск". */
  label: string;
  onPress: () => void;
  disabled?: boolean;
  /** Longer explanation for the help overlay. */
  description?: string;
}

/** Matches a key spec against a keyboard event. Letters and digits use `code`, so Cyrillic layouts work. */
export function matchesKey(e: KeyboardEvent, key: string): boolean {
  if (key === '?') return e.key === '?' || (e.code === 'Slash' && e.shiftKey);
  if (e.shiftKey) return false;
  if (key === '/') return e.key === '/' || e.code === 'Slash';
  if (/^\d$/.test(key)) return e.code === `Digit${key}`;
  if (/^[a-z]$/i.test(key)) return e.code === `Key${key.toUpperCase()}`;
  return e.key === key;
}

/** Hotkeys must not fire while the user types. */
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName);
}
