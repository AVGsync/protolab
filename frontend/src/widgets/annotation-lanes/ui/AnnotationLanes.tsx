import type { ReactNode, Ref } from 'react';
import type { FramingRule } from '@/entities/interpretation';
import { fieldRange, MessageStatusMark, type FieldLink, type Message } from '@/entities/message';
import type { StreamGap, StreamSegment } from '@/entities/stream';
import type { Direction } from '@/shared/api';
import { cn, countLabel, rangeEnd, useElementWidth, type ByteRange } from '@/shared/lib';
import { fieldLaneClass } from '@/shared/ui';
import { fieldLane, frameLane, rulerTicks, tcpLane, type LaneSegment } from '../model/lanes';

export interface AnnotationLanesProps {
  message: Message;
  segments: StreamSegment[];
  gaps: StreamGap[];
  framing?: FramingRule;
  channel: Direction;
  /** Selection in the directed stream. */
  selection: ByteRange | null;
  link: FieldLink | null;
  /** A click on a segment selects its bytes (stream offsets). */
  onSelect: (range: ByteRange) => void;
  onLink: (link: FieldLink | null) => void;
}

/** Approximate width of a 11px mono glyph, to decide whether a label fits into its segment. */
const GLYPH = 6.8;

/**
 * Decoded structure of the selected message above the bytes, like the protocol lanes of a logic analyzer:
 * TCP segments, frame, fields. A click on a segment selects its bytes.
 */
export function AnnotationLanes({ message, segments, gaps, framing, channel, selection, link, onSelect, onLink }: AnnotationLanesProps) {
  const [trackRef, width] = useElementWidth<HTMLDivElement>();
  const tcp = tcpLane(message, segments, gaps);
  const lanes: { name: string; items: LaneSegment[] }[] = [
    { name: 'TCP', items: tcp },
    { name: 'Кадр', items: frameLane(framing, message.length) },
    { name: 'Поля', items: fieldLane(message) },
  ].filter((lane) => lane.items.length > 0);
  const total = Math.max(1, message.length);
  const ticks = rulerTicks(total, width);
  const packets = tcp.filter((s) => s.kind !== 'gap').length;

  const stream = (s: LaneSegment): ByteRange => fieldRange(message, s);
  const ring = channel === 'c2s' ? 'shadow-[inset_0_0_0_1.5px_var(--pl-ch-c2s)]' : 'shadow-[inset_0_0_0_1.5px_var(--pl-ch-s2c)]';

  return (
    <section className="shrink-0 border-b border-line bg-surface px-4 pt-3 pb-3.5">
      <h3 className="mb-2 flex items-baseline gap-2.5 text-sm font-semibold">
        #{message.index} {message.ruleTitle ?? message.ruleId ?? 'не распознано'}
        <span className="font-mono text-xs font-normal text-muted">
          {message.length} Б, {countLabel(packets, ['TCP-сегмент', 'TCP-сегмента', 'TCP-сегментов'])}
        </span>
        <MessageStatusMark status={message.status} />
      </h3>
      <div className="grid grid-cols-[44px_minmax(0,1fr)] items-center gap-y-1">
        <span />
        <div className="relative h-3.5 font-mono text-2xs text-muted" aria-hidden>
          {ticks.map((o) => (
            <span
              key={o}
              className="absolute top-0"
              style={o === total - 1 && ticks.length > 1 ? { right: 0 } : { left: `${(o / total) * 100}%` }}
            >
              {o}
            </span>
          ))}
        </div>
        {lanes.map(({ name, items }, laneIndex) => (
          <LaneRow key={name} name={name} trackRef={laneIndex === 0 ? trackRef : undefined}>
            {items.map((s) => {
              const range = stream(s);
              const selected = selection !== null && selection.offset <= range.offset && rangeEnd(selection) >= rangeEnd(range);
              const linked = link !== null && s.path !== undefined && link.messageIndex === message.index && link.path === s.path;
              const px = (s.length / total) * width;
              const fits = px >= s.label.length * GLYPH + 12;
              return (
                <button
                  key={s.key}
                  type="button"
                  title={s.title}
                  aria-label={s.title}
                  onClick={() => onSelect(range)}
                  onMouseEnter={s.path ? () => onLink({ messageIndex: message.index, path: s.path!, range }) : undefined}
                  onMouseLeave={s.path ? () => onLink(null) : undefined}
                  className={cn(
                    'absolute inset-y-0 flex min-w-[3px] items-center overflow-hidden border-l border-bg font-mono text-2xs whitespace-nowrap',
                    s.tone !== undefined ? fieldLaneClass(s.tone) : 'bg-surface-2 shadow-[inset_0_0_0_1px_var(--pl-line)]',
                    s.kind === 'gap' && 'bg-hatch text-unknown',
                    s.kind === 'violation' && 'bg-danger text-bg',
                    selected ? ring : linked && 'shadow-[inset_0_0_0_1px_var(--pl-text)]',
                  )}
                  style={{ left: `${(s.offset / total) * 100}%`, width: `${(s.length / total) * 100}%` }}
                >
                  {fits && <span className="px-1.5">{s.label}</span>}
                </button>
              );
            })}
          </LaneRow>
        ))}
      </div>
    </section>
  );
}

function LaneRow({ name, trackRef, children }: { name: string; trackRef?: Ref<HTMLDivElement>; children: ReactNode }) {
  return (
    <>
      <span className="text-xs text-muted">{name}</span>
      <div ref={trackRef} className="relative h-5">
        {children}
      </div>
    </>
  );
}
