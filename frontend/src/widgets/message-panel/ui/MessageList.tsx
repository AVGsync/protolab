import { useVirtualizer } from '@tanstack/react-virtual';
import { useEffect, useRef } from 'react';
import { MessageStatusMark, type Message } from '@/entities/message';
import { DirectionLabel } from '@/entities/session';
import { cn, formatTimestamp } from '@/shared/lib';

export interface MessageListProps {
  messages: Message[];
  activeIndex: number | null;
  onSelect: (message: Message) => void;
  /** Sizes the scroll area, e.g. `max-h-64`. */
  className?: string;
}

const ROW_HEIGHT = 46;

/** Messages in time order, both directions: ordinal, channel-coloured direction, rule, time and size. Virtualized. */
export function MessageList({ messages, activeIndex, onSelect, className }: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: messages.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  });

  // Selecting bytes in the stream moves the active message: keep it in view.
  useEffect(() => {
    const at = messages.findIndex((m) => m.index === activeIndex);
    if (at >= 0) virtualizer.scrollToIndex(at, { align: 'auto' });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- follow the active message only
  }, [activeIndex]);

  return (
    <div ref={scrollRef} className={cn('overflow-y-auto', className)}>
      <ul style={{ height: virtualizer.getTotalSize() }} className="relative">
        {virtualizer.getVirtualItems().map((v) => {
          const m = messages[v.index];
          return (
            <li key={m.index} className="absolute inset-x-0" style={{ top: v.start, height: ROW_HEIGHT }}>
              <button
                type="button"
                onClick={() => onSelect(m)}
                aria-current={activeIndex === m.index}
                className={cn(
                  'grid h-full w-full grid-cols-[32px_48px_minmax(0,1fr)_auto] items-center gap-2 border-b border-line px-4 text-left hover:bg-surface-2',
                  activeIndex === m.index && 'bg-surface-2',
                )}
              >
                <span className="font-mono text-xs text-muted">#{m.index}</span>
                <DirectionLabel direction={m.direction} compact />
                <span className="min-w-0">
                  <span className="block truncate">{m.ruleId ?? <span className="text-muted">не распознано</span>}</span>
                  <span className="block font-mono text-2xs leading-4 text-muted">
                    {formatTimestamp(m.timestamp, 3)} @{m.offset} {m.length} Б
                  </span>
                </span>
                <MessageStatusMark status={m.status} />
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
