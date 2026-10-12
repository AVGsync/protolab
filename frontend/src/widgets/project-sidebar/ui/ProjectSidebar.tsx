import { useQuery } from '@tanstack/react-query';
import {
  Binary,
  ClipboardList,
  Columns3,
  FileCode2,
  FileOutput,
  FlaskConical,
  HardDriveDownload,
  LayoutDashboard,
  Lightbulb,
  Network,
  NotebookPen,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { NavLink } from 'react-router';
import { projectQueries, type ProjectStats } from '@/entities/project';
import { routes } from '@/shared/config';
import { cn } from '@/shared/lib';

interface NavItem {
  to: string;
  label: string;
  icon: ReactNode;
  count?: keyof ProjectStats;
  end?: boolean;
}

interface NavGroup {
  title: string;
  items: NavItem[];
}

function groups(projectId: string): NavGroup[] {
  return [
    {
      title: 'Данные',
      items: [
        { to: routes.project(projectId), label: 'Обзор', icon: <LayoutDashboard size={15} />, end: true },
        { to: routes.captures(projectId), label: 'Записи трафика', icon: <HardDriveDownload size={15} />, count: 'captures' },
        { to: routes.sessions(projectId), label: 'Сессии', icon: <Network size={15} />, count: 'sessions' },
        { to: routes.actions(projectId), label: 'Журнал действий', icon: <ClipboardList size={15} />, count: 'actions' },
      ],
    },
    {
      title: 'Исследование',
      items: [
        { to: routes.compare(projectId), label: 'Сравнение', icon: <Columns3 size={15} /> },
        { to: routes.interpretations(projectId), label: 'Интерпретации', icon: <FileCode2 size={15} />, count: 'interpretations' },
        { to: routes.hypotheses(projectId), label: 'Гипотезы', icon: <Lightbulb size={15} />, count: 'hypotheses' },
        { to: routes.observations(projectId), label: 'Наблюдения', icon: <NotebookPen size={15} />, count: 'observations' },
      ],
    },
    {
      title: 'Результат',
      items: [
        { to: routes.verification(projectId), label: 'Проверка', icon: <FlaskConical size={15} />, count: 'runs' },
        { to: routes.reports(projectId), label: 'Отчёты и экспорт', icon: <FileOutput size={15} /> },
      ],
    },
  ];
}

export function ProjectSidebar({ projectId }: { projectId: string }) {
  const { data: project } = useQuery(projectQueries.detail(projectId));
  return (
    <nav className="flex w-56 shrink-0 flex-col gap-4 overflow-y-auto border-r border-line bg-surface/60 px-2 py-4">
      {groups(projectId).map((group) => (
        <div key={group.title}>
          <p className="mb-1 px-2.5 text-[10.5px] font-semibold tracking-wider text-subtle uppercase">{group.title}</p>
          <ul className="flex flex-col gap-0.5">
            {group.items.map((item) => (
              <li key={item.to}>
                <NavLink
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) =>
                    cn(
                      'flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors',
                      isActive ? 'bg-accent/15 text-fg shadow-[inset_2px_0_0_var(--color-accent)]' : 'text-muted hover:bg-surface-2 hover:text-fg',
                    )
                  }
                >
                  {item.icon}
                  <span className="flex-1">{item.label}</span>
                  {item.count && project && <span className="font-mono text-[11px] text-subtle">{project.stats[item.count]}</span>}
                </NavLink>
              </li>
            ))}
          </ul>
        </div>
      ))}
      <div className="mt-auto flex items-center gap-2 px-2.5 text-[11px] text-subtle">
        <Binary size={12} /> TCP/IPv4 · PCAP/PCAPNG
      </div>
    </nav>
  );
}
