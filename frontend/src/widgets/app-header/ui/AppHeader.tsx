import { useQuery } from '@tanstack/react-query';
import { MoonIcon, SunIcon } from '@phosphor-icons/react';
import { Link, NavLink, useParams } from 'react-router';
import { projectQueries } from '@/entities/project';
import { env, PROJECT_SECTIONS, routes } from '@/shared/config';
import { cn, useTheme } from '@/shared/lib';
import { Tag } from '@/shared/ui';
import { JobsIndicator } from './JobsIndicator';

/** A square probe glyph; the only place besides the heat scale where the brand gradient appears. */
function Logo() {
  return (
    <svg viewBox="0 0 14 14" className="size-3.5" aria-hidden>
      <defs>
        <linearGradient id="pl-mark" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="var(--pl-accent)" />
          <stop offset="1" stopColor="var(--pl-accent-2)" />
        </linearGradient>
      </defs>
      <path fill="url(#pl-mark)" fillRule="evenodd" d="M0 0h14v14H0zM4.2 4.2v5.6h5.6V4.2z" />
    </svg>
  );
}

function ThemeToggle() {
  const { theme, toggle } = useTheme();
  return (
    <button
      type="button"
      onClick={toggle}
      title={theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}
      aria-label={theme === 'dark' ? 'Включить светлую тему' : 'Включить тёмную тему'}
      className="inline-flex size-6 items-center justify-center rounded-ctl text-muted hover:bg-surface-2 hover:text-fg"
    >
      {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
    </button>
  );
}

/** Top bar: logo, project name, six numbered sections, jobs, theme and the system page. */
export function AppHeader() {
  const { projectId } = useParams();
  const project = useQuery({ ...projectQueries.detail(projectId ?? ''), enabled: !!projectId });

  return (
    <header className="relative flex h-10 shrink-0 items-center gap-5 bg-surface px-4 after:absolute after:inset-x-0 after:bottom-0 after:h-px after:bg-line">
      <Link to={routes.home()} className="flex items-center gap-2 font-semibold" title="К списку проектов">
        <Logo />
        ProtoLab
      </Link>
      {projectId && (
        <>
          <Link to={routes.project(projectId)} className="max-w-64 truncate text-muted hover:text-fg">
            {project.data?.name ?? projectId}
          </Link>
          <nav className="flex h-full items-stretch">
            {PROJECT_SECTIONS.map((section) => (
              <NavLink
                key={section.id}
                to={section.to(projectId).split('?')[0]}
                title={`${section.label} (${section.key})`}
                className={({ isActive }) =>
                  cn(
                    'relative z-10 flex items-center gap-1.5 px-2.5 transition-colors',
                    'after:absolute after:inset-x-0 after:bottom-0 after:h-0.5',
                    isActive ? 'text-fg after:bg-accent' : 'text-muted hover:text-fg',
                  )
                }
              >
                {section.label}
                <span className="font-mono text-[11px] opacity-70">{section.key}</span>
              </NavLink>
            ))}
          </nav>
        </>
      )}
      <div className="ml-auto flex items-center gap-4 text-xs text-muted">
        {env.useMocks && <Tag title="API эмулируется в браузере (MSW). Данные сбрасываются при перезагрузке.">mock</Tag>}
        <JobsIndicator />
        <ThemeToggle />
        <NavLink to={routes.system()} className={({ isActive }) => cn('hover:text-fg', isActive && 'text-fg')}>
          Система
        </NavLink>
      </div>
    </header>
  );
}
