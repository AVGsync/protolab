import { Status, Tag } from '@/shared/ui';
import type { CaptureIntegrity, CaptureStatus } from '../model/types';

/** A ready capture is the normal state and stays silent. */
export function CaptureStatusMark({ status }: { status: CaptureStatus }) {
  if (status === 'ready') return null;
  return status === 'indexing' ? <Status tone="info">индексация</Status> : <Status tone="danger">ошибка</Status>;
}

/** Intact files are silent; a changed file is a fault. */
export function IntegrityMark({ integrity }: { integrity: CaptureIntegrity }) {
  if (integrity === 'ok') return null;
  if (integrity === 'mismatch') return <Status tone="danger" title="Файл изменён после импорта">изменён после импорта</Status>;
  return <Tag title="SHA-256 ещё не сверялся с записанным при импорте">не проверен</Tag>;
}
