import { useQuery } from '@tanstack/react-query';
import { MoonIcon, SunIcon } from '@phosphor-icons/react';
import { Link, NavLink, useParams } from 'react-router';
import { projectQueries } from '@/entities/project';
import { env, PROJECT_SECTIONS, routes } from '@/shared/config';
import { cn, useTheme } from '@/shared/lib';
import { Tag } from '@/shared/ui';
import { JobsIndicator } from './JobsIndicator';

/**
 * The mark is the app's own sign for "bytes we do not understand yet": a square hatched like the unknown
 * bytes of the hex view (see bg-hatch). Flat, one outline colour, the accent only on the hatching.
 */
function Logo() {
  return (
    <svg viewBox="0 0 14 14" className="size-3.5 shrink-0" fill="none" aria-hidden>
      <rect x="0.75" y="0.75" width="12.5" height="12.5" stroke="currentColor" strokeWidth="1.5" />
      <path d="M2.5 8.5l6-6M2.5 11.5l9-9M5.5 11.5l6-6" stroke="var(--pl-accent)" strokeWidth="1.25" />
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
      <Link to={routes.home()} className="flex items-center gap-2 font-semibold tracking-tight" title="К списку проектов">
        <Logo />
        ProtoLab
      </Link>
      {projectId && (
        <>
          <span aria-hidden className="h-4 w-px bg-line" />
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
                <span className="font-mono text-2xs text-muted">{section.key}</span>
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
