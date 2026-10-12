import { Crosshair } from 'lucide-react';
import type { ActionEntry } from '@/entities/action';
import { EndpointPair, SessionFlagBadges, SessionStatusBadge, type TcpSession } from '@/entities/session';
import { diffMs, formatBytes, formatDuration, formatTimestamp } from '@/shared/lib';
import { Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export interface SessionTableProps {
  sessions: TcpSession[];
  actions?: ActionEntry[];
  onOpen: (session: TcpSession) => void;
}

export function SessionTable({ sessions, actions = [], onOpen }: SessionTableProps) {
  const actionById = new Map(actions.map((a) => [a.id, a]));
  return (
    <Table>
      <THead>
        <tr>
          <Th>Сессия</Th>
          <Th>Соединение</Th>
          <Th>Начало (UTC)</Th>
          <Th className="text-right">Длит.</Th>
          <Th className="text-right">Пакеты</Th>
          <Th className="text-right">C→S / S→C</Th>
          <Th>Действие</Th>
          <Th>Состояние</Th>
        </tr>
      </THead>
      <TBody>
        {sessions.map((s) => (
          <Tr key={s.id} onClick={() => onOpen(s)}>
            <Td>
              <span className="inline-flex items-center gap-1.5 font-mono text-[12px]">
                {s.isTarget && <Crosshair size={12} className="text-glow" aria-label="целевая" />}
                {s.id}
              </span>
              <div className="text-[11px] text-subtle">
                {s.capture.fileName} · tcp.stream {s.index}
              </div>
            </Td>
            <Td>
              <EndpointPair client={s.client} server={s.server} />
            </Td>
            <Td className="font-mono text-[12px] whitespace-nowrap text-muted">
              {s.startedAt.slice(0, 10)} {formatTimestamp(s.startedAt, 3)}
            </Td>
            <Td className="text-right font-mono text-[12px] text-muted">{formatDuration(diffMs(s.startedAt, s.endedAt))}</Td>
            <Td className="text-right font-mono text-[12px]">{s.packetCount}</Td>
            <Td className="text-right font-mono text-[12px] whitespace-nowrap">
              {formatBytes(s.bytes.c2s)} / {formatBytes(s.bytes.s2c)}
            </Td>
            <Td className="max-w-56 truncate text-[12px]">
              {s.actionIds.map((id) => actionById.get(id)?.label).filter(Boolean).join(', ') || <span className="text-subtle">—</span>}
            </Td>
            <Td>
              <div className="flex flex-wrap items-center gap-1">
                <SessionStatusBadge status={s.status} />
                <SessionFlagBadges flags={s.flags} roles={s.roles} />
              </div>
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
