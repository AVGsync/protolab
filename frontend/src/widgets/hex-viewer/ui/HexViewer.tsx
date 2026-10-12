import { useVirtualizer } from '@tanstack/react-virtual';
import { useEffect, useMemo, useRef, useState, type MouseEvent } from 'react';
import type { Message } from '@/entities/message';
import type { StreamGap, StreamSegment } from '@/entities/stream';
import type { Direction } from '@/shared/api';
import { cn, rangeBetween, rangeEnd, type ByteRange } from '@/shared/lib';
import { BYTES_PER_ROW } from '../model/row';
import { buildGapLabels, buildStreamIndex } from '../model/streamIndex';
import { ASCII_WIDTH, CELL_WIDTH, GROUP_GAP, GUTTER_WIDTH, HexRow, OFFSET_WIDTH, ROW_HEIGHT, ROW_MIN_WIDTH } from './HexRow';

export interface HexViewerProps {
  bytes: Uint8Array;
  /** 1 = unknown byte (capture gap). Rendered as "??", never as a value. */
  unknownMask: Uint8Array;
  gaps: StreamGap[];
  segments: StreamSegment[];
  /** Messages of this directed stream: their fields are underlined, their boundaries ruled. */
  messages: Message[];
  /** Index of the message shown in the lanes; the others are underlined fainter. */
  activeMessage?: number;
  /** Search hits: dashed outline. */
  hits?: ByteRange[];
  selection: ByteRange | null;
  /** Bytes of the field under the pointer in another pane. */
  link?: ByteRange | null;
  /** The byte the pointer is on (or the selection start): outlined in the channel colour. */
  cursor: number | null;
  /** Direction of the stream: colours the cursor and the edge stripe. */
  channel: Direction;
  onSelectionChange: (range: ByteRange | null) => void;
  onCursorChange: (offset: number | null) => void;
  /** Change `nonce` to scroll to `offset` again. */
  scrollTo?: { offset: number; nonce: number };
  className?: string;
}

const NO_HITS: ByteRange[] = [];

function clip(range: ByteRange | null | undefined, rowStart: number): [number, number] {
  if (!range) return [-1, -1];
  const rowEnd = rowStart + BYTES_PER_ROW;
  return range.offset < rowEnd && rangeEnd(range) > rowStart ? [Math.max(range.offset, rowStart), Math.min(rangeEnd(range), rowEnd)] : [-1, -1];
}

/**
 * Virtualized hex view of a directed stream: 16 bytes per row in 8+8 columns, fields as underlines,
 * message boundaries as rules, capture gaps as hatching. Selection by drag or Shift+click.
 */
export function HexViewer({
  bytes,
  unknownMask,
  gaps,
  segments,
  messages,
  activeMessage = -1,
  hits = NO_HITS,
  selection,
  link,
  cursor,
  channel,
  onSelectionChange,
  onCursorChange,
  scrollTo,
  className,
}: HexViewerProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [draft, setDraftState] = useState<ByteRange | null>(null);
  const draftRef = useRef<ByteRange | null>(null);
  const anchor = useRef<number | null>(null);
  const setDraft = (range: ByteRange | null) => {
    draftRef.current = range;
    setDraftState(range);
  };

  const index = useMemo(() => buildStreamIndex(bytes.length, segments, hits), [bytes.length, segments, hits]);
  const gapLabels = useMemo(() => buildGapLabels(gaps), [gaps]);

  const rows = Math.ceil(bytes.length / BYTES_PER_ROW);
  const virtualizer = useVirtualizer({
    count: rows,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 24,
  });

  useEffect(() => {
    if (scrollTo && rows > 0) {
      virtualizer.scrollToIndex(Math.min(rows - 1, Math.floor(scrollTo.offset / BYTES_PER_ROW)), { align: 'center' });
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

  const lastCursor = useRef<number | null>(null);
  const onMouseMove = (e: MouseEvent) => {
    const o = offsetFromEvent(e);
    if (o === null) return;
    if (o !== lastCursor.current) {
      lastCursor.current = o;
      onCursorChange(o);
    }
    if (anchor.current !== null) setDraft(rangeBetween(anchor.current, o));
  };

  const shown = draft ?? selection;
  const dimUncovered = messages.length > 0;

  return (
    <div className={cn('relative flex min-h-0 flex-col overflow-x-auto bg-bg', className)}>
      <div
        aria-hidden
        className={cn('pointer-events-none absolute inset-y-0 left-0 z-10 w-0.5', channel === 'c2s' ? 'bg-ch-c2s' : 'bg-ch-s2c')}
      />
      {bytes.length === 0 ? (
        <p className="px-6 py-10 text-center text-muted">В этом направлении данных нет.</p>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col" style={{ minWidth: ROW_MIN_WIDTH }}>
          <HexHeader />
          <div
            ref={scrollRef}
            className="min-h-0 flex-1 overflow-y-auto font-mono text-sm select-none"
            onMouseDown={onMouseDown}
            onMouseMove={onMouseMove}
            onMouseLeave={() => {
              lastCursor.current = null;
              onCursorChange(null);
            }}
          >
            <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
              {virtualizer.getVirtualItems().map((v) => {
                const rowStart = v.index * BYTES_PER_ROW;
                const [selFrom, selTo] = clip(shown, rowStart);
                const [linkFrom, linkTo] = clip(link, rowStart);
                return (
                  <HexRow
                    key={v.key}
                    row={v.index}
                    top={v.start}
                    bytes={bytes}
                    unknown={unknownMask}
                    messages={messages}
                    index={index}
                    gapLabels={gapLabels.get(v.index)}
                    dimUncovered={dimUncovered}
                    activeMessage={activeMessage}
                    selFrom={selFrom}
                    selTo={selTo}
                    linkFrom={linkFrom}
                    linkTo={linkTo}
                    cursor={cursor !== null && cursor >= rowStart && cursor < rowStart + BYTES_PER_ROW ? cursor : -1}
                    channel={channel}
                  />
                );
              })}
            </div>
            <p
              className="flex h-8 items-center gap-2 font-sans text-2xs text-muted"
              style={{ paddingLeft: 12 + GUTTER_WIDTH + OFFSET_WIDTH }}
            >
              <span aria-hidden className="h-px w-6 bg-line-strong" />
              конец потока, {bytes.length} Б
            </p>
          </div>
        </div>
      )}
    </div>
  );
}

const COLUMNS = Array.from({ length: BYTES_PER_ROW }, (_, k) => k);

function HexHeader() {
  return (
    <div
      className="flex shrink-0 items-center border-b border-line pl-3 font-mono text-2xs whitespace-pre text-muted"
      style={{ height: ROW_HEIGHT }}
    >
      <span className="shrink-0" style={{ width: GUTTER_WIDTH }} />
      <span className="shrink-0" style={{ width: OFFSET_WIDTH }}>
        смещ.
      </span>
      <span className="shrink-0">
        {COLUMNS.map((k) => (
          <span key={k} className="inline-block text-center" style={{ width: CELL_WIDTH, marginLeft: k === 8 ? GROUP_GAP : 0 }}>
            {k.toString(16).toUpperCase()}
          </span>
        ))}
      </span>
      <span className="ml-4 shrink-0" style={{ width: BYTES_PER_ROW * ASCII_WIDTH }}>
        ASCII
      </span>
    </div>
  );
}
