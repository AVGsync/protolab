import { Status, type Tone } from '@/shared/ui';
import type { SegmentFlag } from '../model/types';

const FLAGS: Record<SegmentFlag, { tone: Tone; label: string; title: string }> = {
  retransmission: { tone: 'info', label: 'ретрансмиссия', title: 'Повтор уже полученных байтов: в поток не добавлен' },
  out_of_order: { tone: 'info', label: 'вне порядка', title: 'Пришёл раньше предшествующего сегмента; поток собран по seq' },
  overlap_conflict: { tone: 'danger', label: 'конфликт', title: 'Повтор с другими байтами: выбор по политике проекта' },
  checksum_warning: { tone: 'muted', label: 'checksum', title: 'Неверная контрольная сумма TCP: offloading на узле захвата или повреждение (см. диагностику сессии)' },
  truncated: { tone: 'warn', label: 'усечён', title: 'Пакет обрезан snaplen: хвост неизвестен' },
  keep_alive: { tone: 'muted', label: 'keep-alive', title: 'Проба TCP keep-alive: её байт не относится к данным и в поток не добавлен' },
};

export function SegmentFlagMark({ flag }: { flag: SegmentFlag }) {
  const f = FLAGS[flag];
  return (
    <Status tone={f.tone} title={f.title}>
      {f.label}
    </Status>
  );
}
