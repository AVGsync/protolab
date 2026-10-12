import { useQuery } from '@tanstack/react-query';
import { Outlet } from 'react-router';
import { projectQueries } from '@/entities/project';
import { useRequiredParams } from '@/shared/lib';
import { TabNav } from '@/shared/ui';
import type { SectionTabs } from './section-tabs';

/** A project section with sub-tabs: the tab strip above, the active sub-page below. */
export function SectionLayout({ tabs }: { tabs: SectionTabs }) {
  const { projectId } = useRequiredParams('projectId');
  const { data: project } = useQuery(projectQueries.detail(projectId));
  return (
    <div className="flex h-full min-h-0 flex-col">
      <TabNav items={tabs(projectId, project?.stats)} className="shrink-0 bg-surface" />
      <div className="min-h-0 flex-1 overflow-auto">
        <Outlet />
      </div>
    </div>
  );
}
