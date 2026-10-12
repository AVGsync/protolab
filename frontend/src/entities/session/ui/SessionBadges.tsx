import { ArrowLeft, ArrowRight } from 'lucide-react';
import type { Direction, Endpoint } from '@/shared/api';
import { cn } from '@/shared/lib';
import { Badge } from '@/shared/ui';
import { DIRECTION_LABEL } from '../model/labels';
import type { RoleCertainty, SessionFlags, SessionStatus } from '../model/types';

const STATUS = {
  complete: { tone: 'success', label: 'полная' },
  partial: { tone: 'warning', label: 'неполная' },
  error: { tone: 'danger', label: 'ошибка' },
} as const;

export function SessionStatusBadge({ status }: { status: SessionStatus }) {
  const s = STATUS[status];
  return (
    <Badge tone={s.tone} dot>
      {s.label}
    </Badge>
  );
}

export function SessionFlagBadges({ flags, roles }: { flags: SessionFlags; roles?: RoleCertainty }) {
  return (
    <span className="inline-flex flex-wrap gap-1">
      {!flags.handshake && (
        <Badge tone="warning" title="Начало соединения не попало в запись">
          без SYN
        </Badge>
      )}
      {roles === 'unknown' && <Badge tone="warning">роли не определены</Badge>}
      {roles === 'inferred' && (
        <Badge tone="neutral" title="Роли определены по порту, а не по рукопожатию">
          роли по порту
        </Badge>
      )}
      {flags.gaps > 0 && <Badge tone="danger">пропуски: {flags.gaps}</Badge>}
      {flags.overlapConflicts > 0 && <Badge tone="danger">конфликт перекрытий: {flags.overlapConflicts}</Badge>}
      {flags.retransmissions > 0 && <Badge tone="info">ретрансмиссии: {flags.retransmissions}</Badge>}
      {flags.outOfOrder > 0 && <Badge tone="info">вне порядка: {flags.outOfOrder}</Badge>}
      {flags.checksumWarnings > 0 && (
        <Badge tone="neutral" title="Возможно checksum offloading на узле захвата">
          checksum: {flags.checksumWarnings}
        </Badge>
      )}
    </span>
  );
}

export function EndpointPair({ client, server, className }: { client: Endpoint; server: Endpoint; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1.5 font-mono text-[12px]', className)}>
      <span>
        {client.ip}:{client.port}
      </span>
      <ArrowRight size={12} className="text-subtle" />
      <span className="text-fg">
        {server.ip}:<span className="text-glow">{server.port}</span>
      </span>
    </span>
  );
}

export function DirectionBadge({ direction, compact }: { direction: Direction; compact?: boolean }) {
  return (
    <Badge tone={direction === 'c2s' ? 'info' : 'violet'} icon={direction === 'c2s' ? <ArrowRight size={11} /> : <ArrowLeft size={11} />}>
      {compact ? (direction === 'c2s' ? 'C→S' : 'S→C') : DIRECTION_LABEL[direction]}
    </Badge>
  );
}
