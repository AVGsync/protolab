import { firstMessageEndingAfter, type Message, type ParsedField } from '@/entities/message';
import { fieldTone } from '@/shared/ui';

export const BYTES_PER_ROW = 16;

/** Per-byte flags of a row. */
export const COVERED = 1;
/** First and last byte of a field (or of an array element): the underline is inset there. */
export const FIELD_START = 2;
export const FIELD_END = 4;
/** Odd array element: its underline is fainter. */
export const ALT = 8;
export const VIOLATION = 16;

export interface RowCells {
  flags: Uint8Array;
  /** Field tone 0..3 of the top-level field. */
  tone: Uint8Array;
  /** Index of the message that covers the byte, -1 when none. */
  message: Int32Array;
  /** Messages that start in this row. */
  starts: { at: number; index: number; violated: boolean }[];
}

/**
 * Fields and message boundaries of one hex row.
 * Computed per visible row from the sorted messages of the directed stream, so a huge stream costs nothing up front.
 */
export function buildRowCells(row: number, messages: readonly Message[]): RowCells {
  const rowStart = row * BYTES_PER_ROW;
  const rowEnd = rowStart + BYTES_PER_ROW;
  const cells: RowCells = {
    flags: new Uint8Array(BYTES_PER_ROW),
    tone: new Uint8Array(BYTES_PER_ROW),
    message: new Int32Array(BYTES_PER_ROW).fill(-1),
    starts: [],
  };

  const paint = (from: number, to: number, apply: (i: number, first: boolean, last: boolean) => void) => {
    const a = Math.max(from, rowStart);
    const b = Math.min(to, rowEnd);
    for (let o = a; o < b; o++) apply(o - rowStart, o === from, o === to - 1);
  };

  const markViolations = (m: Message, fields: readonly ParsedField[]) => {
    for (const f of fields) {
      if (f.children?.length) markViolations(m, f.children);
      else if (f.status === 'violation') paint(m.offset + f.offset, m.offset + f.offset + f.length, (i) => (cells.flags[i] |= VIOLATION));
    }
  };

  for (let k = firstMessageEndingAfter(messages, rowStart); k < messages.length && messages[k].offset < rowEnd; k++) {
    const m = messages[k];
    paint(m.offset, m.offset + m.length, (i) => (cells.message[i] = m.index));
    if (m.offset >= rowStart) {
      cells.starts.push({ at: m.offset - rowStart, index: m.index, violated: m.status === 'violation' });
    }

    m.fields.forEach((f, fi) => {
      const from = m.offset + f.offset;
      const to = from + f.length;
      const tone = fieldTone(fi);
      paint(from, to, (i, first, last) => {
        cells.flags[i] |= COVERED | (first ? FIELD_START : 0) | (last ? FIELD_END : 0);
        cells.tone[i] = tone;
      });
      // Array elements get their own underline segments, alternating in strength.
      f.children?.forEach((c, ci) => {
        const cFrom = m.offset + c.offset;
        paint(cFrom, cFrom + c.length, (i, first, last) => {
          cells.flags[i] |= (first ? FIELD_START : 0) | (last ? FIELD_END : 0) | (ci % 2 === 1 ? ALT : 0);
        });
      });
    });
    markViolations(m, m.fields);
  }
  return cells;
}
