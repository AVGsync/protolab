import type { HypothesisStatus } from './types';

export const HYPOTHESIS_STATUS: Record<
  HypothesisStatus,
  { label: string; tone: 'neutral' | 'info' | 'success' | 'danger'; description: string }
> = {
  proposed: { label: 'предложена', tone: 'neutral', description: 'Есть наблюдение, проверка не начиналась' },
  testing: { label: 'проверяется', tone: 'info', description: 'Собираются примеры и контрпримеры' },
  supported: {
    label: 'подтверждается',
    tone: 'success',
    description: 'Согласуется с корпусом; совпадение не является доказательством',
  },
  refuted: { label: 'опровергнута', tone: 'danger', description: 'Найден контрпример' },
};

export const HYPOTHESIS_STATUSES = Object.keys(HYPOTHESIS_STATUS) as HypothesisStatus[];
