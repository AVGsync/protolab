import type { ProjectStats } from '@/entities/project';
import { routes } from '@/shared/config';
import type { TabNavItem } from '@/shared/ui';

export type SectionTabs = (projectId: string, stats?: ProjectStats) => TabNavItem[];

/** 1. Данные */
export const dataTabs: SectionTabs = (id, stats) => [
  { to: routes.data(id), label: 'Записи', count: stats?.captures, end: true },
  { to: routes.actions(id), label: 'Журнал действий', count: stats?.actions },
];

/** 4. Спецификация */
export const specTabs: SectionTabs = (id, stats) => [
  { to: routes.spec(id), label: 'Редактор', end: true },
  { to: routes.hypotheses(id), label: 'Гипотезы', count: stats?.hypotheses },
  { to: routes.observations(id), label: 'Наблюдения', count: stats?.observations },
];
