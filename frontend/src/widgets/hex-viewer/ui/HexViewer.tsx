import { useVirtualizer } from '@tanstack/react-virtual';
import { memo, useEffect, useMemo, useRef, useState, type MouseEvent } from 'react';
import type { StreamSegment } from '@/entities/stream';
import { cn, formatOffset, formatTimestamp, hexByte, isPrintable, rangeBetween, type ByteRange } from '@/shared/lib';

export interface HexHighlight extends ByteRange {
  /** Any CSS color; rendered with transparency. */
  color: string;
  label?: string;
}

export interface HexViewerProps {
  bytes: Uint8Array;
  /** 1 = unknown byte (capture gap). Rendered as "??", never as a value. */
  unknownMask: Uint8Array;
  segments: StreamSegment[];
  messageStarts?: number[];
  highlights?: HexHighlight[];
  selection: ByteRange | null;
  onSelectionChange: (range: ByteRange | null) => void;
  /** Change `nonce` to scroll to `offset` again. */
  scrollTo?: { offset: number; nonce: number };
  bytesPerRow?: 8 | 16 | 32;
  className?: string;
}

const ROW_HEIGHT = 20;

interface Lookup {
  segIndex: Int32Array;
  msgStart: Uint8Array;
  hlIndex: Int16Array;
}

function buildLookup(length: number, segments: StreamSegment[], messageStarts: number[], highlights: HexHighlight[]): Lookup {
  const segIndex = new Int32Array(length).fill(-1);
  segments.forEach((s, i) => segIndex.fill(i, Math.max(0, s.offset), Math.min(length, s.offset + s.length)));
  const msgStart = new Uint8Array(length);
  for (const o of messageStarts) if (o >= 0 && o < length) msgStart[o] = 1;
  const hlIndex = new Int16Array(length).fill(-1);
  highlights.forEach((h, i) => hlIndex.fill(i, Math.max(0, h.offset), Math.min(length, h.offset + h.length)));
  return { segIndex, msgStart, hlIndex };
}

export function HexViewer({
  bytes,
  unknownMask,
  segments,
  messageStarts = [],
  highlights = [],
  selection,
  onSelectionChange,
  scrollTo,
  bytesPerRow = 16,
  className,
}: HexViewerProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const [draft, setDraftState] = useState<ByteRange | null>(null);
  const draftRef = useRef<ByteRange | null>(null);
  const anchor = useRef<number | null>(null);
  const setDraft = (range: ByteRange | null) => {
    draftRef.current = range;
    setDraftState(range);
  };

  const lookup = useMemo(
    () => buildLookup(bytes.length, segments, messageStarts, highlights),
    [bytes.length, segments, messageStarts, highlights],
  );

  const rows = Math.ceil(bytes.length / bytesPerRow);
  const virtualizer = useVirtualizer({
    count: rows,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 24,
  });

  useEffect(() => {
    if (scrollTo && rows > 0) {
      virtualizer.scrollToIndex(Math.min(rows - 1, Math.floor(scrollTo.offset / bytesPerRow)), { align: 'center' });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- scroll only when a new target is requested
  }, [scrollTo?.nonce]);

  // Finish a drag even if the mouse is released outside of the viewer.
  useEffect(() => {
    const up = () => {
      if (anchor.current === null) return;
      const finished = draftRef.current;
      anchor.current = null;
      draftRef.current = null;
      setDraftState(null);
      if (finished) onSelectionChange(finished);
    };
    document.addEventListener('mouseup', up);
    return () => document.removeEventListener('mouseup', up);
  }, [onSelectionChange]);

  const offsetFromEvent = (e: MouseEvent): number | null => {
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-o]');
    return el ? Number(el.dataset.o) : null;
  };

  const onMouseDown = (e: MouseEvent) => {
    const o = offsetFromEvent(e);
    if (o === null || e.button !== 0) return;
    e.preventDefault();
    if (e.shiftKey && selection) {
      onSelectionChange(rangeBetween(selection.offset, o));
      return;
    }
    anchor.current = o;
    setDraft({ offset: o, length: 1 });
  };

  const onMouseMove = (e: MouseEvent) => {
    const o = offsetFromEvent(e);
    if (o === null) return;
    setHover(o);
    if (anchor.current !== null) setDraft(rangeBetween(anchor.current, o));
  };

  const active = draft ?? selection;
  const hoverSeg = hover !== null ? segments[lookup.segIndex[hover]] : undefined;

  return (
    <div className={cn('flex min-h-0 flex-col bg-bg', className)}>
      <div
        ref={scrollRef}
        className="min-h-0 flex-1 overflow-auto font-mono text-[12px] leading-5 select-none"
        onMouseDown={onMouseDown}
        onMouseMove={onMouseMove}
        onMouseLeave={() => setHover(null)}
      >
        {bytes.length === 0 ? (
          <p className="p-6 font-sans text-muted">Поток пуст — в этом направлении данных нет.</p>
        ) : (
          <div style={{ height: virtualizer.getTotalSize(), position: 'relative', minWidth: 'max-content' }}>
            {virtualizer.getVirtualItems().map((v) => (
              <HexRow
                key={v.key}
                row={v.index}
                top={v.start}
                bytes={bytes}
                bytesPerRow={bytesPerRow}
                unknown={unknownMask}
                lookup={lookup}
                highlights={highlights}
                selection={active}
                hover={hover}
              />
            ))}
          </div>
        )}
      </div>
      <div className="flex h-7 shrink-0 items-center gap-4 border-t border-line bg-surface px-3 font-mono text-[11px] text-muted">
        {hover !== null ? (
          <>
            <span>
              <span className="text-subtle">offset</span> {formatOffset(hover)} ({hover})
            </span>
            <span>
              <span className="text-subtle">byte</span>{' '}
              {unknownMask[hover] ? <span className="text-danger">?? неизвестен</span> : `0x${hexByte(bytes[hover])} = ${bytes[hover]}`}
            </span>
            {hoverSeg && (
              <span>
                <span className="text-subtle">пакет</span> #{hoverSeg.packetNumber} · {formatTimestamp(hoverSeg.timestamp)}
                {hoverSeg.flags.length > 0 && <span className="text-warn"> · {hoverSeg.flags.join(', ')}</span>}
              </span>
            )}
            {lookup.hlIndex[hover] >= 0 && highlights[lookup.hlIndex[hover]].label && (
              <span className="text-fg">{highlights[lookup.hlIndex[hover]].label}</span>
            )}
          </>
        ) : (
          <span>
            {bytes.length} Б · выделение: перетаскивание или Shift+клик
          </span>
        )}
        {active && (
          <span className="ml-auto text-fg">
            выделено {formatOffset(active.offset)}+{active.length}
          </span>
        )}
      </div>
    </div>
  );
}

interface HexRowProps {
  row: number;
  top: number;
  bytes: Uint8Array;
  bytesPerRow: number;
  unknown: Uint8Array;
  lookup: Lookup;
  highlights: HexHighlight[];
  selection: ByteRange | null;
  hover: number | null;
}

const HexRow = memo(function HexRow({ row, top, bytes, bytesPerRow, unknown, lookup, highlights, selection, hover }: HexRowProps) {
  const start = row * bytesPerRow;
  const end = Math.min(bytes.length, start + bytesPerRow);
  const hexCells = [];
  const asciiCells = [];

  for (let o = start; o < start + bytesPerRow; o++) {
    if (o >= end) {
      hexCells.push(<span key={o} className="inline-block w-[22px]" />);
      asciiCells.push(<span key={o} className="inline-block w-[8px]" />);
      continue;
    }
    const isUnknown = unknown[o] === 1;
    const selected = selection !== null && o >= selection.offset && o < selection.offset + selection.length;
    const hl = lookup.hlIndex[o];
    const seg = lookup.segIndex[o];
    const style = !selected && hl >= 0 ? { backgroundColor: `color-mix(in srgb, ${highlights[hl].color} 32%, transparent)` } : undefined;
    const cls = cn(
      'inline-block cursor-text text-center',
      seg >= 0 && seg % 2 === 1 && 'bg-surface-2',
      seg < 0 && !isUnknown && 'text-subtle',
      isUnknown && 'bg-hatch text-danger/80',
      lookup.msgStart[o] === 1 && 'shadow-[inset_2px_0_0_var(--color-line-strong)]',
      selected && 'bg-selection text-fg',
      hover === o && 'outline outline-1 -outline-offset-1 outline-glow',
    );
    const gapAfter = (o - start) % 8 === 7 && o - start < bytesPerRow - 1;
    hexCells.push(
      <span key={o} data-o={o} className={cn(cls, 'w-[22px]', gapAfter && 'mr-2')} style={style}>
        {isUnknown ? '??' : hexByte(bytes[o])}
      </span>,
    );
    const b = bytes[o];
    asciiCells.push(
      <span key={o} data-o={o} className={cn(cls, 'w-[8px]', !isUnknown && !isPrintable(b) && 'text-subtle')} style={style}>
        {isUnknown ? '·' : isPrintable(b) ? String.fromCharCode(b) : '.'}
      </span>,
    );
  }

  return (
    <div className="absolute left-0 flex h-5 items-center whitespace-pre" style={{ top }}>
      <span className="w-[76px] shrink-0 pl-3 text-subtle">{start.toString(16).toUpperCase().padStart(6, '0')}</span>
      <span className="pr-4">{hexCells}</span>
      <span className="border-l border-line pr-4 pl-3">{asciiCells}</span>
    </div>
  );
});
