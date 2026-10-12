import { useEffect, useId, useRef } from 'react';
import type { ContextKey } from './keys';
import { useSoftKeysStore } from './store';

/**
 * Registers page actions in the soft-key footer and binds their hotkeys while the page is mounted.
 * Handlers may change on every render; only the key, label and disabled state re-register.
 */
export function useContextKeys(keys: readonly ContextKey[]): void {
  const id = useId();
  const latest = useRef(keys);
  useEffect(() => {
    latest.current = keys;
  });

  const signature = keys.map((k) => `${k.key}|${k.label}|${k.disabled ? 1 : 0}|${k.description ?? ''}`).join('\n');
  useEffect(() => {
    const bound = latest.current.map<ContextKey>((k) => ({
      ...k,
      onPress: () => latest.current.find((x) => x.key === k.key)?.onPress(),
    }));
    useSoftKeysStore.getState().register(id, bound);
    return () => useSoftKeysStore.getState().unregister(id);
  }, [id, signature]);
}
