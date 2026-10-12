import { useQuery } from '@tanstack/react-query';
import { FileCode2 } from 'lucide-react';
import { Link } from 'react-router';
import { interpretationQueries } from '@/entities/interpretation';
import { CreateInterpretationButton } from '@/features/edit-interpretation';
import { routes } from '@/shared/config';
import { formatDateTime, useRequiredParams } from '@/shared/lib';
import { Card, EmptyState, ErrorState, Loader, Page, PageHeader } from '@/shared/ui';

export function InterpretationsPage() {
  const { projectId } = useRequiredParams('projectId');
  const { data, isLoading, error, refetch } = useQuery(interpretationQueries.list(projectId));

  return (
    <Page>
      <PageHeader
        title="Интерпретации"
        description="Декларативное машиночитаемое описание протокола (protolab/v1): кадрирование, типы сообщений, поля, проверки по журналу действий. Каждое изменение — новая неизменяемая версия."
        actions={<CreateInterpretationButton projectId={projectId} />}
      />
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && (
        <EmptyState icon={<FileCode2 size={20} />} title="Интерпретаций нет" description="Создайте первую версию — даже частичное описание полезно, если границы известного обозначены." />
      )}
      <div className="grid gap-3 md:grid-cols-2">
        {data?.items.map((i) => (
          <Link key={i.id} to={routes.interpretation(projectId, i.id)}>
            <Card className="px-4 py-4 transition-colors hover:border-line-strong hover:bg-surface-2">
              <div className="flex items-center justify-between">
                <h3 className="font-semibold">{i.name}</h3>
                <span className="rounded bg-surface-3 px-1.5 font-mono text-[11px]">v{i.latestVersion}</span>
              </div>
              <p className="mt-1 text-[12.5px] text-muted">{i.description || 'Без описания'}</p>
              <p className="mt-3 text-[11px] text-subtle">Изменена {formatDateTime(i.updatedAt)}</p>
            </Card>
          </Link>
        ))}
      </div>
    </Page>
  );
}
