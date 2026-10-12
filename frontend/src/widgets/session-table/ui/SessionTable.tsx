import type { ActionEntry } from '@/entities/action';
import { DirectionLabel, EndpointPair, SessionAnomalyMark, sessionAnomalies, SessionStatusMark, type TcpSession } from '@/entities/session';
import { diffMs, formatBytes, formatDuration, formatTimestamp } from '@/shared/lib';
import { Table, Tag, TBody, Td, Th, THead, Tr } from '@/shared/ui';

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
          <Th className="text-right">
            <span className="inline-flex items-center gap-2">
              <DirectionLabel direction="c2s" compact />
              <DirectionLabel direction="s2c" compact />
            </span>
          </Th>
          <Th>Действие</Th>
          <Th />
        </tr>
      </THead>
      <TBody>
        {sessions.map((s) => (
          <Tr key={s.id} onClick={() => onOpen(s)}>
            <Td>
              <span
                className="inline-flex items-center gap-2 font-mono text-xs"
                title={`tcp.stream ${s.index}, ${s.capture.fileName}`}
              >
                {s.id}
                {!s.isTarget && <Tag title="Сессия не на целевом порту проекта">фон</Tag>}
              </span>
            </Td>
            <Td>
              <EndpointPair client={s.client} server={s.server} />
            </Td>
            <Td className="font-mono text-xs whitespace-nowrap text-muted">
              {s.startedAt.slice(0, 10)} {formatTimestamp(s.startedAt, 3)}
            </Td>
            <Td className="text-right font-mono text-xs whitespace-nowrap text-muted">{formatDuration(diffMs(s.startedAt, s.endedAt))}</Td>
            <Td className="text-right font-mono text-xs">{s.packetCount}</Td>
            <Td className="text-right font-mono text-xs whitespace-nowrap">
              <span className="text-ch-c2s">{formatBytes(s.bytes.c2s)}</span> / <span className="text-ch-s2c">{formatBytes(s.bytes.s2c)}</span>
            </Td>
            <Td className="max-w-56 truncate text-xs">
              {s.actionIds.map((id) => actionById.get(id)?.label).filter(Boolean).join(', ') || <span className="text-muted">—</span>}
            </Td>
            <Td>
              {sessionAnomalies(s.flags, s.roles, s.diagnostics).length > 0 ? (
                <SessionAnomalyMark flags={s.flags} roles={s.roles} diagnostics={s.diagnostics} />
              ) : (
                <SessionStatusMark status={s.status} />
              )}
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
