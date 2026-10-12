import { useQuery } from '@tanstack/react-query';
import { projectQueries } from '@/entities/project';
import { reportQueries, type ArtifactKind } from '@/entities/report';
import { ExportProjectButton } from '@/features/export-project';
import { GenerateReportButton } from '@/features/generate-report';
import { ProjectSettingsButton } from '@/features/project-settings';
import { formatBytes, formatDateTime, shortHash, useRequiredParams } from '@/shared/lib';
import {
  CopyButton,
  DownloadButton,
  Disclosure,
  EmptyState,
  ErrorState,
  Loader,
  Page,
  PageHeader,
  Pane,
  PaneHeader,
  Readout,
  Status,
} from '@/shared/ui';

const ARTIFACT_LABEL: Record<ArtifactKind, string> = {
  report_html: 'Отчёт (HTML)',
  report_md: 'Читаемый отчёт',
  interpretation: 'Интерпретация (машиночитаемая)',
  results: 'Результаты проверок',
  examples: 'Примеры и ожидаемые результаты',
};

export function ReportsPage() {
  const { projectId } = useRequiredParams('projectId');
  const project = useQuery(projectQueries.detail(projectId));
  const { data, isLoading, error, refetch } = useQuery(reportQueries.list(projectId));
  const settings = project.data?.settings;

  return (
    <Page>
      <PageHeader
        title="Отчёт"
        actions={
          <>
            <ExportProjectButton projectId={projectId} />
            <GenerateReportButton projectId={projectId} />
          </>
        }
      />


      {project.data && settings && (
        <div className="mb-4 flex items-center justify-between gap-4 border-y border-line py-2">
          <Readout
            items={[
              { label: 'перекрытия', value: settings.overlapPolicy === 'first' ? 'первый пакет' : 'последний пакет' },
              { label: 'checksum TCP', value: settings.checksumPolicy === 'warn' ? 'предупреждать' : 'игнорировать' },
              { label: 'порты', value: settings.targetPorts.join(', ') || '—' },
            ]}
          />
          <ProjectSettingsButton project={project.data} />
        </div>
      )}

      <Disclosure summary="Что входит в архив проекта" className="mb-4">
        <p className="border-l border-line pl-4 text-xs text-muted">
          Архив содержит записи, интерпретации, наблюдения и проверки; пути относительные. При открытии SHA-256 записей сверяются, устаревшие
          результаты помечаются.
        </p>
      </Disclosure>

      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && <EmptyState title="Отчётов ещё нет" />}
      <div className="flex flex-col gap-4">
        {data?.items.map((r) => (
          <Pane key={r.id}>
            <PaneHeader
              title={r.title}
              actions={
                <>
                  <span className="font-mono text-xs text-muted">
                    {r.interpretationName} v{r.version}, {formatDateTime(r.createdAt)}
                  </span>
                  {r.status === 'pending' && <Status tone="info">формируется</Status>}
                  {r.status === 'failed' && <Status tone="danger">ошибка</Status>}
                </>
              }
            />
            {r.status === 'ready' && (
              <div className="flex flex-col gap-3 p-3">
                <ul className="flex flex-col">
                  {r.artifacts.map((a) => (
                    <li key={a.fileName} className="border-b border-line last:border-b-0">
                      <DownloadButton
                        href={a.url}
                        fileName={a.fileName}
                        className="flex w-full items-center gap-4 py-1.5 text-left hover:bg-surface-2"
                      >
                        <span className="w-64 shrink-0">{ARTIFACT_LABEL[a.kind]}</span>
                        <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted">
                          {a.fileName}, {formatBytes(a.sizeBytes)}, {shortHash(a.sha256, 10)}
                        </span>
                        <span className="text-xs text-muted">скачать</span>
                      </DownloadButton>
                    </li>
                  ))}
                </ul>
                <div>
                  <div className="mb-1 flex items-center justify-between">
                    <p className="text-xs text-muted">Повторное применение к другим данным</p>
                    <CopyButton value={r.reproduceCommand} label="копировать" />
                  </div>
                  <pre className="overflow-x-auto border border-line bg-bg px-3 py-2 font-mono text-xs">{r.reproduceCommand}</pre>
                </div>
              </div>
            )}
          </Pane>
        ))}
      </div>
    </Page>
  );
}
