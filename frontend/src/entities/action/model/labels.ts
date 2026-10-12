import type { ActionKind } from './types';

export const ACTION_KIND_LABEL: Record<ActionKind, string> = {
  read_params: 'Чтение параметров',
  set_param: 'Изменение настройки',
  get_measurement: 'Получение измерения',
  read_log: 'Чтение журнала',
  other: 'Другое',
};

/** Numeric parameters usable as a reference value when comparing messages. */
export function numericParams(params: Record<string, string | number> | undefined): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [k, v] of Object.entries(params ?? {})) {
    const n = typeof v === 'number' ? v : Number(v);
    if (v !== '' && Number.isFinite(n)) out[k] = n;
  }
  return out;
}
