import type { ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { useSoftKeysStore } from '../model/store';

/** Renders its children into the right side of the soft-key footer (live readout of a page). */
export function FooterReadout({ children }: { children: ReactNode }) {
  const el = useSoftKeysStore((s) => s.readoutEl);
  return el ? createPortal(children, el) : null;
}
