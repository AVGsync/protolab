import { projectApi } from '@/entities/project';
import { DownloadButton } from '@/shared/ui';

/** Portable bundle: original captures, interpretations, observations, runs; relative paths only. */
export function ExportProjectButton({ projectId, variant = 'secondary' }: { projectId: string; variant?: 'secondary' | 'primary' }) {
  return (
    <DownloadButton href={projectApi.exportUrl(projectId)} fileName={`${projectId}.protolab.zip`} variant={variant}>
      Экспорт проекта
    </DownloadButton>
  );
}
