import { useQuery } from '@tanstack/react-query';
import { Download, FileJson, FileText, PackageCheck } from 'lucide-react';
import { reportQueries, type ArtifactKind } from '@/entities/report';
import { ExportProjectButton } from '@/features/export-project';
import { GenerateReportButton } from '@/features/generate-report';
import { formatBytes, formatDateTime, shortHash, useRequiredParams } from '@/shared/lib';
import { Badge, Card, CardBody, CardHeader, CopyButton, DownloadButton, EmptyState, ErrorState, Loader, Page, PageHeader } from '@/shared/ui';

const STATUS_LABEL = { pending: 'формируется', ready: 'готов', failed: 'ошибка' } as const;

const ARTIFACT_LABEL: Record<ArtifactKind, string> = {
  report_html: 'Отчёт (HTML)',
  report_md: 'Читаемый отчёт',
  interpretation: 'Интерпретация (машиночитаемая)',
  results: 'Результаты проверок',
  examples: 'Примеры и ожидаемые результаты',
};

export function ReportsPage() {
  const { projectId } = useRequiredParams('projectId');
  const { data, isLoading, error, refetch } = useQuery(reportQueries.list(projectId));

  return (
    <Page>
      <PageHeader
        title="Отчёты и передача"
        description="Отчёт содержит основания выводов, проверенные примеры, область применимости, контрпримеры и открытые вопросы. Интерпретация применяется к новым данным без повторной ручной работы."
        actions={
          <>
            <ExportProjectButton projectId={projectId} />
            <GenerateReportButton projectId={projectId} />
          </>
        }
      />

      <Card className="mb-5">
        <CardHeader icon={<PackageCheck size={16} />} title="Перенос проекта" />
        <CardBody className="grid gap-3 text-[12.5px] text-muted md:grid-cols-3">
          <p>Архив проекта содержит исходные записи, интерпретации, наблюдения, связи с источниками и результаты проверок.</p>
          <p>Пути внутри проекта относительные: проект открывается в другом каталоге или на другой машине без обращения к прежним путям.</p>
          <p>При открытии SHA-256 записей сверяются; устаревшие результаты помечаются, а не выдаются за актуальные.</p>
        </CardBody>
      </Card>

      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && <EmptyState icon={<FileText size={20} />} title="Отчётов ещё нет" />}
      <div className="flex flex-col gap-4">
        {data?.items.map((r) => (
          <Card key={r.id}>
            <CardHeader
              title={r.title}
              description={`${r.interpretationName} v${r.version} · проверки: ${r.runIds.join(', ') || '—'} · ${formatDateTime(r.createdAt)}`}
              actions={<Badge tone={r.status === 'ready' ? 'success' : r.status === 'failed' ? 'danger' : 'info'}>{STATUS_LABEL[r.status]}</Badge>}
            />
            {r.status === 'ready' && (
              <CardBody className="flex flex-col gap-4">
                <div className="grid gap-2 md:grid-cols-2">
                  {r.artifacts.map((a) => (
                    <DownloadButton
                      key={a.fileName}
                      href={a.url}
                      fileName={a.fileName}
                      className="flex items-center gap-3 rounded-md border border-line bg-bg px-3 py-2.5 text-left hover:border-line-strong hover:bg-surface-2"
                    >
                      {a.kind.startsWith('report') ? <FileText size={16} className="text-glow" /> : <FileJson size={16} className="text-info" />}
                      <div className="min-w-0 flex-1">
                        <p className="text-[12.5px]">{ARTIFACT_LABEL[a.kind]}</p>
                        <p className="truncate font-mono text-[11px] text-subtle">
                          {a.fileName} · {formatBytes(a.sizeBytes)} · {shortHash(a.sha256, 10)}
                        </p>
                      </div>
                      <Download size={14} className="text-muted" />
                    </DownloadButton>
                  ))}
                </div>
                <div>
                  <div className="mb-1.5 flex items-center justify-between">
                    <p className="text-[11px] font-semibold tracking-wider text-subtle uppercase">Повторное применение к другим данным</p>
                    <CopyButton value={r.reproduceCommand} label="копировать" />
                  </div>
                  <pre className="overflow-x-auto rounded-md border border-line bg-bg px-3 py-2.5 font-mono text-[12px] text-fg/90">
                    {r.reproduceCommand}
                  </pre>
                </div>
              </CardBody>
            )}
          </Card>
        ))}
      </div>
    </Page>
  );
}
