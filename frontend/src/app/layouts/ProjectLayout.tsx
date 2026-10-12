import { Outlet } from 'react-router';
import { ProjectSidebar } from '@/widgets/project-sidebar';
import { useRequiredParams } from '@/shared/lib';

export function ProjectLayout() {
  const { projectId } = useRequiredParams('projectId');
  return (
    <div className="flex h-full min-h-0">
      <ProjectSidebar projectId={projectId} />
      <div className="min-h-0 min-w-0 flex-1 overflow-auto">
        <Outlet />
      </div>
    </div>
  );
}
