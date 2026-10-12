import type { StreamGap, StreamSegment } from '@/entities/stream';
import type { ByteRange } from '@/shared/lib';
import { BYTES_PER_ROW } from './row';

/** Bit flags of the per-byte stream index. */
export const SEGMENT_START = 1;
/** The segment that starts here is retransmitted, reordered, truncated or has a bad checksum. */
export const SEGMENT_FLAGGED = 2;
export const HIT = 4;
export const HIT_START = 8;
export const HIT_END = 16;

/** TCP segment starts and search hits as one byte per stream byte. */
export function buildStreamIndex(length: number, segments: readonly StreamSegment[], hits: readonly ByteRange[]): Uint8Array {
  const index = new Uint8Array(length);
  for (const s of segments) {
    if (s.offset >= 0 && s.offset < length) index[s.offset] |= SEGMENT_START | (s.flags.length > 0 ? SEGMENT_FLAGGED : 0);
  }
  for (const h of hits) {
    const from = Math.max(0, h.offset);
    const to = Math.min(length, h.offset + h.length);
    if (to <= from) continue;
    for (let o = from; o < to; o++) index[o] |= HIT;
    index[from] |= HIT_START;
    index[to - 1] |= HIT_END;
  }
  return index;
}

export interface GapLabel {
  /** Byte of the row where the gap starts. */
  at: number;
  /** Cells of the gap inside this row. */
  cells: number;
  text: string;
}

const GAP_TEXT = { lost: 'потеря', truncated: 'усечение' } as const;

export const gapText = (gap: StreamGap): string => `${GAP_TEXT[gap.reason]} ${gap.length} Б`;

/** Gap labels by the row in which the gap starts: shown once, in the ASCII column. */
export function buildGapLabels(gaps: readonly StreamGap[]): Map<number, GapLabel[]> {
  const byRow = new Map<number, GapLabel[]>();
  for (const gap of gaps) {
    if (gap.length <= 0) continue;
    const row = Math.floor(gap.offset / BYTES_PER_ROW);
    const at = gap.offset - row * BYTES_PER_ROW;
    const label: GapLabel = { at, cells: Math.min(gap.length, BYTES_PER_ROW - at), text: gapText(gap) };
    byRow.set(row, [...(byRow.get(row) ?? []), label]);
  }
  return byRow;
}
