import { routes } from './routes';

export type SectionId = 'data' | 'sessions' | 'compare' | 'spec' | 'verification' | 'report';

export interface ProjectSection {
  id: SectionId;
  /** Hotkey: the same digits select the same sections in the TUI. */
  key: string;
  label: string;
  to: (projectId: string) => string;
}

/** The six numbered sections of a project, in workflow order. */
export const PROJECT_SECTIONS: readonly ProjectSection[] = [
  { id: 'data', key: '1', label: 'Данные', to: routes.data },
  { id: 'sessions', key: '2', label: 'Сессии', to: (id) => routes.sessions(id) },
  { id: 'compare', key: '3', label: 'Сравнение', to: routes.compare },
  { id: 'spec', key: '4', label: 'Спецификация', to: (id) => routes.spec(id) },
  { id: 'verification', key: '5', label: 'Проверка', to: routes.verification },
  { id: 'report', key: '6', label: 'Отчёт', to: routes.report },
];
