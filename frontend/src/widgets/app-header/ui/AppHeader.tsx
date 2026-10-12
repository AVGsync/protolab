import { useQuery } from '@tanstack/react-query';
import { ChevronRight, Cpu } from 'lucide-react';
import { Link, NavLink, useParams } from 'react-router';
import { projectQueries } from '@/entities/project';
import { env, routes } from '@/shared/config';
import { cn } from '@/shared/lib';
import { Badge } from '@/shared/ui';
import { JobsIndicator } from './JobsIndicator';

function Logo() {
  return (
    <svg viewBox="0 0 32 32" className="size-6" aria-hidden>
      <rect width="32" height="32" rx="7" className="fill-accent-strong" />
      <g fill="none" stroke="#f6a04d" strokeWidth="2.2" strokeLinecap="round">
        <circle cx="16" cy="16" r="9" />
        <path d="M16 7v5M16 20v5M7 16h5M20 16h5" />
      </g>
      <circle cx="16" cy="16" r="2.6" fill="#f6a04d" />
    </svg>
  );
}

export function AppHeader() {
  const { projectId } = useParams();
  const project = useQuery({ ...projectQueries.detail(projectId ?? ''), enabled: !!projectId });

  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-line bg-surface px-4">
      <Link to={routes.home()} className="flex items-center gap-2.5">
        <Logo />
        <span className="font-semibold tracking-tight">ProtoLab</span>
        <span className="hidden text-xs text-subtle md:inline">Лаборатория протоколов</span>
      </Link>
      {projectId && (
        <>
          <ChevronRight size={14} className="text-subtle" />
          <Link to={routes.project(projectId)} className="truncate text-[13px] hover:text-glow">
            {project.data?.name ?? projectId}
          </Link>
        </>
      )}
      <div className="ml-auto flex items-center gap-2">
        {env.useMocks && (
          <Badge tone="warning" title="API эмулируется в браузере (MSW). Данные сбрасываются при перезагрузке.">
            mock API
          </Badge>
        )}
        <JobsIndicator />
        <NavLink
          to={routes.system()}
          title="Система и ресурсы"
          className={({ isActive }) =>
            cn('inline-flex size-8 items-center justify-center rounded-md border border-line text-muted hover:text-fg', isActive && 'text-fg')
          }
        >
          <Cpu size={14} />
        </NavLink>
      </div>
    </header>
  );
}
