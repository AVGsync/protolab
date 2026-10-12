import { create } from 'zustand';
import type { ContextKey } from './keys';

interface SoftKeysState {
  /** Keys registered by the mounted pages, in registration order. */
  groups: { id: string; keys: ContextKey[] }[];
  helpOpen: boolean;
  /** Right side of the footer; pages render their live readout into it. */
  readoutEl: HTMLElement | null;
  register: (id: string, keys: ContextKey[]) => void;
  unregister: (id: string) => void;
  setHelpOpen: (open: boolean) => void;
  setReadoutEl: (el: HTMLElement | null) => void;
}

export const useSoftKeysStore = create<SoftKeysState>((set) => ({
  groups: [],
  helpOpen: false,
  readoutEl: null,
  register: (id, keys) =>
    set((s) =>
      s.groups.some((g) => g.id === id)
        ? { groups: s.groups.map((g) => (g.id === id ? { id, keys } : g)) }
        : { groups: [...s.groups, { id, keys }] },
    ),
  unregister: (id) => set((s) => ({ groups: s.groups.filter((g) => g.id !== id) })),
  setHelpOpen: (helpOpen) => set({ helpOpen }),
  setReadoutEl: (readoutEl) => set({ readoutEl }),
}));
