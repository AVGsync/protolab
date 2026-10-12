import { SegmentFlagBadge, type Packet } from '@/entities/stream';
import { DirectionBadge } from '@/entities/session';
import { diffMs, formatDuration } from '@/shared/lib';
import { Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export interface PacketTableProps {
  packets: Packet[];
  onSelect?: (packet: Packet) => void;
}

/** Packets of a session in capture order with TCP flags and reassembly annotations. */
export function PacketTable({ packets, onSelect }: PacketTableProps) {
  const t0 = packets[0]?.timestamp;
  return (
    <Table>
      <THead>
        <tr>
          <Th>#</Th>
          <Th>Время</Th>
          <Th />
          <Th>Флаги</Th>
          <Th className="text-right">Seq</Th>
          <Th className="text-right">Len</Th>
          <Th className="text-right">@поток</Th>
          <Th>Пометки</Th>
        </tr>
      </THead>
      <TBody>
        {packets.map((p) => (
          <Tr key={p.number} onClick={onSelect && p.payloadLength > 0 ? () => onSelect(p) : undefined}>
            <Td className="font-mono text-[11.5px] text-subtle">{p.number}</Td>
            <Td className="font-mono text-[11.5px] whitespace-nowrap text-muted">+{t0 ? formatDuration(diffMs(t0, p.timestamp)) : ''}</Td>
            <Td>
              <DirectionBadge direction={p.direction} compact />
            </Td>
            <Td className="font-mono text-[11px] whitespace-nowrap">{p.tcpFlags.join(',')}</Td>
            <Td className="text-right font-mono text-[11.5px] text-muted">{p.seq}</Td>
            <Td className="text-right font-mono text-[11.5px]">{p.payloadLength || ''}</Td>
            <Td className="text-right font-mono text-[11.5px] text-info">{p.streamOffset ?? ''}</Td>
            <Td>
              <span className="flex flex-wrap gap-1">
                {p.flags.map((f) => (
                  <SegmentFlagBadge key={f} flag={f} />
                ))}
              </span>
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
