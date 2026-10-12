import { ShieldAlert, ShieldCheck } from 'lucide-react';
import { Badge } from '@/shared/ui';
import type { CaptureIntegrity, CaptureStatus } from '../model/types';

const STATUS = {
  indexing: { tone: 'info', label: 'Индексация' },
  ready: { tone: 'success', label: 'Готово' },
  error: { tone: 'danger', label: 'Ошибка' },
} as const;

export function CaptureStatusBadge({ status }: { status: CaptureStatus }) {
  const s = STATUS[status];
  return (
    <Badge tone={s.tone} dot>
      {s.label}
    </Badge>
  );
}

export function IntegrityBadge({ integrity }: { integrity: CaptureIntegrity }) {
  if (integrity === 'ok')
    return (
      <Badge tone="success" icon={<ShieldCheck size={11} />} title="SHA-256 совпадает с записанным при импорте">
        целостность
      </Badge>
    );
  if (integrity === 'mismatch')
    return (
      <Badge tone="danger" icon={<ShieldAlert size={11} />} title="Файл изменён после импорта">
        изменён!
      </Badge>
    );
  return <Badge tone="neutral">не проверен</Badge>;
}
