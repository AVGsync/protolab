import type { ByteRange } from '@/shared/lib';
import type { Message, ParsedField } from './types';

/** Messages of one directed stream, in stream order. */
export function sortByOffset(messages: readonly Message[]): Message[] {
  return [...messages].sort((a, b) => a.offset - b.offset || a.index - b.index);
}

/** Index of the first message that ends after `offset` (messages must be sorted and must not overlap). */
export function firstMessageEndingAfter(sorted: readonly Message[], offset: number): number {
  let lo = 0;
  let hi = sorted.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (sorted[mid].offset + sorted[mid].length > offset) hi = mid;
    else lo = mid + 1;
  }
  return lo;
}

export function findMessageAt(sorted: readonly Message[], offset: number): Message | undefined {
  const m = sorted[firstMessageEndingAfter(sorted, offset)];
  return m && m.offset <= offset ? m : undefined;
}

/** The innermost field that covers a message-relative offset. */
export function findFieldAt(message: Message, relOffset: number): ParsedField | undefined {
  let found: ParsedField | undefined;
  let level: ParsedField[] | undefined = message.fields;
  while (level) {
    const next: ParsedField | undefined = level.find((f) => relOffset >= f.offset && relOffset < f.offset + f.length);
    if (!next) break;
    found = next;
    level = next.children;
  }
  return found;
}

/** Byte range of a field in the directed stream. */
export const fieldRange = (message: Message, field: Pick<ParsedField, 'offset' | 'length'>): ByteRange => ({
  offset: message.offset + field.offset,
  length: field.length,
});

/** A field under the pointer in one pane: the others (hex, lanes, table) highlight the same bytes. */
export interface FieldLink {
  messageIndex: number;
  path: string;
  /** Bytes of the field in the directed stream. */
  range: ByteRange;
}
