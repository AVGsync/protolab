import { ENCODING_SIZE, INT_ENCODINGS, readInt, type IntEncoding } from './bytes';

export interface CompareRow {
  bytes: Uint8Array;
  /** Known parameter value of the action that produced the message (from the action log). */
  param?: number | null;
}

export type ColumnKind = 'constant' | 'variable' | 'partial';

export interface ColumnStat {
  offset: number;
  kind: ColumnKind;
  /** Number of distinct values among rows that have this offset. */
  distinct: number;
  /** Number of rows long enough to have this offset. */
  present: number;
}

export function columnStats(rows: readonly CompareRow[]): ColumnStat[] {
  const width = rows.reduce((max, r) => Math.max(max, r.bytes.length), 0);
  const stats: ColumnStat[] = [];
  for (let offset = 0; offset < width; offset++) {
    const values = new Set<number>();
    let present = 0;
    for (const r of rows) {
      if (offset < r.bytes.length) {
        present++;
        values.add(r.bytes[offset]);
      }
    }
    const kind: ColumnKind = present < rows.length ? 'partial' : values.size === 1 ? 'constant' : 'variable';
    stats.push({ offset, kind, distinct: values.size, present });
  }
  return stats;
}

/**
 * Variability of a column on the 0..5 heat scale: 0 = the same in every message, 5 = different in every message.
 * Columns missing in some messages have no heat (they are drawn hatched).
 */
export function columnHeat(s: Pick<ColumnStat, 'kind' | 'distinct' | 'present'>): number {
  if (s.kind === 'constant' || s.present < 2) return 0;
  return 1 + Math.round(((s.distinct - 1) / (s.present - 1)) * 4);
}

export interface FieldCandidate {
  offset: number;
  encoding: IntEncoding;
  /** decoded / scale === param */
  scale: number;
  matches: number;
  /** Rows that have a parameter value. */
  total: number;
  /** Indices of rows that contradict the candidate. */
  counterexamples: number[];
}

export interface CandidateSearch {
  candidates: FieldCandidate[];
  /** Explanation when the search is not meaningful. */
  warning?: string;
}

const EPS = 1e-6;

/**
 * Suggests byte positions whose value equals a known action parameter.
 * The result is a hint, not a proof: candidates come with counterexamples
 * and require verification on additional captures.
 */
export function findFieldCandidates(
  rows: readonly CompareRow[],
  opts: { encodings?: readonly IntEncoding[]; scales?: readonly number[]; minRatio?: number } = {},
): CandidateSearch {
  const encodings = opts.encodings ?? INT_ENCODINGS;
  const scales = opts.scales ?? [1, 10, 100];
  const minRatio = opts.minRatio ?? 0.5;

  const withParam = rows
    .map((row, index) => ({ row, index }))
    .filter((r): r is { row: CompareRow & { param: number }; index: number } => typeof r.row.param === 'number');

  if (withParam.length < 2) {
    return { candidates: [], warning: 'Нужно минимум два примера с известным значением параметра.' };
  }
  if (new Set(withParam.map((r) => r.row.param)).size < 2) {
    return {
      candidates: [],
      warning: 'У всех примеров одинаковое значение параметра — различия не с чем сопоставить.',
    };
  }

  const width = withParam.reduce((max, r) => Math.max(max, r.row.bytes.length), 0);
  const candidates: FieldCandidate[] = [];

  for (let offset = 0; offset < width; offset++) {
    for (const encoding of encodings) {
      for (const scale of scales) {
        let matches = 0;
        const counterexamples: number[] = [];
        for (const { row, index } of withParam) {
          const v = readInt(row.bytes, offset, encoding);
          if (v !== null && Math.abs(v - row.param * scale) < EPS) matches++;
          else counterexamples.push(index);
        }
        if (matches >= 2 && matches / withParam.length >= minRatio) {
          candidates.push({ offset, encoding, scale, matches, total: withParam.length, counterexamples });
        }
      }
    }
  }

  candidates.sort(
    (a, b) =>
      b.matches - a.matches ||
      a.scale - b.scale ||
      ENCODING_SIZE[b.encoding] - ENCODING_SIZE[a.encoding] ||
      a.offset - b.offset,
  );

  return { candidates: dedupeSigned(candidates) };
}

/** u16le/i16le with identical matches are the same evidence: keep the first (unsigned) one. */
function dedupeSigned(list: FieldCandidate[]): FieldCandidate[] {
  const seen = new Set<string>();
  const out: FieldCandidate[] = [];
  for (const c of list) {
    const size = ENCODING_SIZE[c.encoding];
    const key = `${c.offset}:${size}:${size > 1 ? c.encoding.slice(-2) : ''}:${c.scale}:${c.matches}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(c);
  }
  return out;
}
