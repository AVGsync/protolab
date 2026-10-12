import type { Direction, Endpoint } from '@/shared/api';
import { cn } from '@/shared/lib';
import { Arrow, Status, Tag } from '@/shared/ui';
import { DIRECTION_LABEL } from '../model/labels';
import { sessionAnomalies } from '../model/anomalies';
import type { RoleCertainty, SessionFlags, SessionStatus } from '../model/types';

/** A complete session is the normal state and stays silent. */
export function SessionStatusMark({ status }: { status: SessionStatus }) {
  if (status === 'complete') return null;
  return status === 'partial' ? <Status tone="warn">неполная</Status> : <Status tone="danger">ошибка</Status>;
}

/** The most severe anomaly of a session (at most one marker per row); the rest is in the tooltip. */
export function SessionAnomalyMark({ flags, roles }: { flags: SessionFlags; roles?: RoleCertainty }) {
  const [first, ...rest] = sessionAnomalies(flags, roles);
  if (!first) return null;
  const title = [first, ...rest].map((a) => `${a.text}: ${a.hint}`).join('\n');
  return (
    <span className="inline-flex items-center gap-1.5" title={title}>
      <Status tone={first.tone}>{first.text}</Status>
      {rest.length > 0 && <Tag>+{rest.length}</Tag>}
    </span>
  );
}

export function EndpointPair({ client, server, className }: { client: Endpoint; server: Endpoint; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1.5 font-mono text-xs text-muted', className)}>
      <span>
        {client.ip}:{client.port}
      </span>
      <Arrow />
      <span className="text-fg">
        {server.ip}:{server.port}
      </span>
    </span>
  );
}

/** Direction is always a channel-coloured stripe plus text: amber for C to S, cyan for S to C. */
export function DirectionLabel({ direction, compact, className }: { direction: Direction; compact?: boolean; className?: string }) {
  const c2s = direction === 'c2s';
  return (
    <span className={cn('inline-flex items-center gap-1.5 whitespace-nowrap', c2s ? 'text-ch-c2s' : 'text-ch-s2c', className)} title={DIRECTION_LABEL[direction]}>
      <i aria-hidden className="h-0.5 w-2.5 shrink-0 bg-current" />
      {compact ? (
        <span className="inline-flex items-center gap-1 font-mono text-[11px]">
          {c2s ? 'C' : 'S'}
          <Arrow />
          {c2s ? 'S' : 'C'}
        </span>
      ) : (
        <span className="inline-flex items-center gap-1.5">
          {c2s ? 'Клиент' : 'Сервер'}
          <Arrow />
          {c2s ? 'сервер' : 'клиент'}
        </span>
      )}
    </span>
  );
}
