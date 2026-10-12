import { memo, useMemo } from 'react';
import type { Message } from '@/entities/message';
import type { Direction } from '@/shared/api';
import { cn, hexByte, isPrintable } from '@/shared/lib';
import { fieldUnderline } from '@/shared/ui';
import { ALT, BYTES_PER_ROW, buildRowCells, COVERED, FIELD_END, FIELD_START, VIOLATION } from '../model/row';
import { HIT, HIT_END, HIT_START, SEGMENT_FLAGGED, SEGMENT_START, type GapLabel } from '../model/streamIndex';

export const ROW_HEIGHT = 22;
export const CELL_WIDTH = 26;
export const GROUP_GAP = 10;
export const ASCII_WIDTH = 8;
/** Gutter (#n marks), offset column and the paddings: keep the header and the rows aligned. */
export const GUTTER_WIDTH = 36;
export const OFFSET_WIDTH = 56;
export const ROW_MIN_WIDTH = 12 + GUTTER_WIDTH + OFFSET_WIDTH + BYTES_PER_ROW * CELL_WIDTH + GROUP_GAP + 16 + BYTES_PER_ROW * ASCII_WIDTH + 12;

export interface HexRowProps {
  row: number;
  top: number;
  bytes: Uint8Array;
  /** 1 = unknown byte (capture gap): shown as "??", never as a value. */
  unknown: Uint8Array;
  /** Sorted messages of this directed stream. */
  messages: readonly Message[];
  /** Segment starts and search hits, see buildStreamIndex. */
  index: Uint8Array;
  gapLabels?: GapLabel[];
  /** Bytes outside every field are muted, but only when messages exist at all. */
  dimUncovered: boolean;
  activeMessage: number;
  /** Selection and link ranges clipped to this row (stream offsets); -1 when they miss the row. */
  selFrom: number;
  selTo: number;
  linkFrom: number;
  linkTo: number;
  cursor: number;
  channel: Direction;
}

const CHANNEL_OUTLINE = { c2s: 'outline-ch-c2s', s2c: 'outline-ch-s2c' } as const;

/** One row of 16 bytes: offset, hex in 8+8 columns with field underlines, ASCII. */
export const HexRow = memo(function HexRow(p: HexRowProps) {
  const start = p.row * BYTES_PER_ROW;
  const cells = useMemo(() => buildRowCells(p.row, p.messages), [p.row, p.messages]);
  const first = cells.starts[0];
  const violated = cells.flags.some((f) => (f & VIOLATION) !== 0);
  const startAt = new Set(cells.starts.map((s) => s.at));

  const hex = [];
  const ascii = [];
  for (let k = 0; k < BYTES_PER_ROW; k++) {
    const o = start + k;
    const groupGap = k === 8 ? 'ml-2.5' : '';
    if (o >= p.bytes.length) {
      hex.push(<span key={k} className={cn('inline-block', groupGap)} style={{ width: CELL_WIDTH }} />);
      continue;
    }
    const unknown = p.unknown[o] === 1;
    const f = cells.flags[k];
    const selected = p.selFrom >= 0 && o >= p.selFrom && o < p.selTo;
    const linked = p.linkFrom >= 0 && o >= p.linkFrom && o < p.linkTo;
    const idx = p.index[o];
    const b = p.bytes[o];

    const shared = cn(
      'relative inline-block text-center leading-[22px]',
      selected ? 'bg-selection' : linked && 'bg-selection/45',
      o === p.cursor && cn('outline-[1.5px] -outline-offset-[1.5px]', CHANNEL_OUTLINE[p.channel]),
    );

    const dimmed = p.activeMessage >= 0 && cells.message[k] !== p.activeMessage;
    const underline =
      !unknown && f & (COVERED | VIOLATION) ? (
        <i
          aria-hidden
          className="absolute bottom-[3px] h-[2px]"
          style={{
            left: f & FIELD_START ? 4 : 2,
            right: f & FIELD_END ? 4 : 2,
            background: f & VIOLATION ? 'var(--pl-danger)' : fieldUnderline(cells.tone[k]),
            opacity: linked ? 1 : (f & ALT ? 0.4 : 0.85) * (dimmed ? 0.55 : 1),
          }}
        />
      ) : null;

    hex.push(
      <span
        key={k}
        data-o={o}
        style={{ width: CELL_WIDTH, height: ROW_HEIGHT }}
        className={cn(
          shared,
          groupGap,
          unknown && 'bg-hatch',
          startAt.has(k) && 'shadow-[inset_1px_0_0_var(--pl-line-strong)]',
          f & VIOLATION ? 'text-danger' : unknown ? 'text-unknown' : f & COVERED || !p.dimUncovered ? 'text-fg' : 'text-muted',
        )}
      >
        {unknown ? '??' : hexByte(b)}
        {underline}
        {idx & SEGMENT_START ? (
          <i aria-hidden className={cn('absolute top-[3px] left-px h-1 w-0.5', idx & SEGMENT_FLAGGED ? 'bg-warn' : 'bg-subtle')} />
        ) : null}
        {idx & HIT ? (
          <i
            aria-hidden
            className={cn(
              'pointer-events-none absolute inset-0 border-y border-dashed border-warn',
              idx & HIT_START && 'border-l',
              idx & HIT_END && 'border-r',
            )}
          />
        ) : null}
      </span>,
    );

    ascii.push(
      <span
        key={k}
        data-o={o}
        style={{ width: ASCII_WIDTH, height: ROW_HEIGHT }}
        className={cn(shared, selected || linked ? 'text-fg' : 'text-muted', f & VIOLATION && 'text-danger')}
      >
        {unknown ? ' ' : isPrintable(b) ? String.fromCharCode(b) : '.'}
      </span>,
    );
  }

  return (
    <div
      className={cn('absolute inset-x-0 flex items-center pl-3 whitespace-pre', first && p.row > 0 && 'shadow-[inset_0_1px_0_var(--pl-line-strong)]')}
      style={{ top: p.top, height: ROW_HEIGHT }}
    >
      <span
        className={cn('shrink-0 pr-2 text-right font-mono text-[10px]', first?.violated || violated ? 'text-danger' : 'text-muted')}
        style={{ width: GUTTER_WIDTH }}
        title={first ? `Сообщение #${first.index} начинается в этой строке` : violated ? 'В строке есть нарушение' : undefined}
      >
        {first ? `#${first.index}` : violated ? '!' : ''}
      </span>
      <span className="shrink-0 text-muted" style={{ width: OFFSET_WIDTH }}>
        {start.toString(16).toUpperCase().padStart(6, '0')}
      </span>
      <span className="shrink-0">{hex}</span>
      <span className="relative ml-4 shrink-0">
        {ascii}
        {p.gapLabels?.map((g) => {
          const fits = g.cells * ASCII_WIDTH >= g.text.length * 7;
          return (
            <span
              key={g.at}
              className="absolute top-0 text-[11px] leading-[22px] text-unknown"
              style={fits ? { left: g.at * ASCII_WIDTH } : { left: BYTES_PER_ROW * ASCII_WIDTH + 8 }}
            >
              {g.text}
            </span>
          );
        })}
      </span>
    </div>
  );
});
