import { SegmentFlagMark, type Packet } from '@/entities/stream';
import { DirectionLabel } from '@/entities/session';
import { cn, diffMs, formatDuration } from '@/shared/lib';

export interface PacketTableProps {
  packets: Packet[];
  onSelect?: (packet: Packet) => void;
}

const th = 'h-7 border-b border-line px-2 font-normal whitespace-nowrap';

/**
 * Packets of a session in capture order: TCP flags, sequence, payload and where it landed in the stream.
 * A packet with something unusual gets one marker line under its row.
 */
export function PacketTable({ packets, onSelect }: PacketTableProps) {
  const t0 = packets[0]?.timestamp;
  return (
    <table className="w-full border-collapse text-left text-xs">
      <thead className="sticky top-0 bg-surface text-xs text-muted">
        <tr>
          <th className={cn(th, 'pl-4 text-right')} title="Номер пакета в записи">
            #
          </th>
          <th className={th}>Время</th>
          <th className={th} />
          <th className={th}>Флаги</th>
          <th className={cn(th, 'text-right')} title="Относительный seq">
            Seq
          </th>
          <th className={cn(th, 'text-right')} title="Байт полезной нагрузки">
            Длина
          </th>
          <th className={cn(th, 'pr-4 text-right')} title="Смещение нагрузки в потоке направления">
            @поток
          </th>
        </tr>
      </thead>
      {packets.map((p) => {
        const clickable = onSelect !== undefined && p.payloadLength > 0;
        const click = clickable ? () => onSelect(p) : undefined;
        const row = cn('group-hover:bg-surface-2');
        return (
          <tbody key={p.number} className={cn('group border-b border-line', clickable && 'cursor-pointer')}>
            <tr onClick={click} className={cn('h-7', clickable && row)}>
              <td className="pl-4 text-right font-mono text-muted">{p.number}</td>
              <td className="px-2 font-mono whitespace-nowrap text-muted">+{t0 ? formatDuration(diffMs(t0, p.timestamp)) : ''}</td>
              <td className="px-2">
                <DirectionLabel direction={p.direction} compact />
              </td>
              <td className="px-2 font-mono whitespace-nowrap">{p.tcpFlags.join(' ')}</td>
              <td className="px-2 text-right font-mono text-muted">{p.seq}</td>
              <td className="px-2 text-right font-mono">{p.payloadLength || ''}</td>
              <td className="pr-4 pl-2 text-right font-mono">{p.streamOffset ?? ''}</td>
            </tr>
            {p.flags[0] && (
              <tr onClick={click} className={cn(clickable && row)}>
                <td />
                <td colSpan={6} className="pb-1.5 pl-2 text-xs">
                  <SegmentFlagMark flag={p.flags[0]} />
                </td>
              </tr>
            )}
          </tbody>
        );
      })}
    </table>
  );
}
