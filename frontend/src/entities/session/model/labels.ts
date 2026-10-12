import type { Direction } from '@/shared/api';

/** Plain-text label (tooltips, selects). In the UI use DirectionLabel, which draws the arrow as SVG. */
export const DIRECTION_LABEL: Record<Direction, string> = {
  c2s: 'Клиент к серверу',
  s2c: 'Сервер к клиенту',
};
