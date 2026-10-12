import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router';
import { projectQueries } from '@/entities/project';
import { CreateProjectButton } from '@/features/create-project';
import { ImportProjectButton } from '@/features/import-project';
import { routes } from '@/shared/config';
import { formatDateTime } from '@/shared/lib';
import { EmptyState, ErrorState, Loader, Page, PageHeader, Pane, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

function Count({ value }: { value: number }) {
  return <Td className={value === 0 ? 'text-right font-mono text-muted' : 'text-right font-mono'}>{value}</Td>;
}

export function ProjectsPage() {
  const { data, isLoading, error, refetch } = useQuery(projectQueries.list());
  const navigate = useNavigate();

  return (
    <Page>
      <PageHeader
        title="Проекты"
        actions={
          <>
            <CreateProjectButton />
            <ImportProjectButton />
          </>
        }
      />
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && (
        <EmptyState
          title="Проектов нет"
          description="Создайте проект и загрузите первую запись трафика."
          action={<CreateProjectButton />}
        />
      )}
      {data && data.items.length > 0 && (
        <Pane>
          <Table>
            <THead>
              <tr>
                <Th>Проект</Th>
                <Th>ID</Th>
                <Th className="text-right">Записи</Th>
                <Th className="text-right">Сессии</Th>
                <Th className="text-right">Гипотезы</Th>
                <Th className="text-right">Проверки</Th>
                <Th className="text-right">Изменён</Th>
              </tr>
            </THead>
            <TBody>
              {data.items.map((p) => (
                <Tr key={p.id} onClick={() => navigate(routes.project(p.id))}>
                  <Td>
                    <div className="flex min-w-0 items-baseline gap-3">
                      <Link to={routes.project(p.id)} className="link shrink-0 font-medium" onClick={(e) => e.stopPropagation()}>
                        {p.name}
                      </Link>
                      <span className="max-w-[48ch] truncate text-muted">{p.description}</span>
                    </div>
                  </Td>
                  <Td className="font-mono text-xs text-muted">{p.id}</Td>
                  <Count value={p.stats.captures} />
                  <Count value={p.stats.sessions} />
                  <Count value={p.stats.hypotheses} />
                  <Count value={p.stats.runs} />
                  <Td className="text-right font-mono text-xs whitespace-nowrap text-muted">{formatDateTime(p.updatedAt)}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        </Pane>
      )}
    </Page>
  );
}
