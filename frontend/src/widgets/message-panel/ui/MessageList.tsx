import type { Message } from '@/entities/message';
import { MessageStatusBadge } from '@/entities/message';
import { DirectionBadge } from '@/entities/session';
import { cn, formatTimestamp } from '@/shared/lib';

export interface MessageListProps {
  messages: Message[];
  activeIndex: number | null;
  onSelect: (message: Message) => void;
}

export function MessageList({ messages, activeIndex, onSelect }: MessageListProps) {
  return (
    <ul className="flex flex-col">
      {messages.map((m) => (
        <li key={m.index}>
          <button
            type="button"
            onClick={() => onSelect(m)}
            className={cn(
              'grid w-full grid-cols-[34px_44px_1fr_auto] items-center gap-2 border-b border-line/60 px-3 py-1.5 text-left hover:bg-surface-2',
              activeIndex === m.index && 'bg-accent/10',
            )}
          >
            <span className="font-mono text-[11px] text-subtle">#{m.index}</span>
            <DirectionBadge direction={m.direction} compact />
            <span className="min-w-0">
              <span className="block truncate text-[12.5px]">{m.ruleId ?? <span className="text-muted">не распознано</span>}</span>
              <span className="font-mono text-[10.5px] text-subtle">
                {formatTimestamp(m.timestamp, 3)} · @{m.offset} · {m.length} Б · пакеты #{m.packetNumbers.join(',')}
              </span>
            </span>
            <MessageStatusBadge status={m.status} />
          </button>
        </li>
      ))}
    </ul>
  );
}
