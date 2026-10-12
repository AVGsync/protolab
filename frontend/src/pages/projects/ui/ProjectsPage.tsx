import { useQuery } from '@tanstack/react-query';
import { FolderOpen } from 'lucide-react';
import { Link } from 'react-router';
import { projectQueries } from '@/entities/project';
import { CreateProjectButton } from '@/features/create-project';
import { ImportProjectButton } from '@/features/import-project';
import { routes } from '@/shared/config';
import { formatDateTime } from '@/shared/lib';
import { Card, EmptyState, ErrorState, Loader, Page } from '@/shared/ui';

const STEPS = [
  ['Записи', 'PCAP/PCAPNG и журнал действий в клиенте'],
  ['Исследование', 'потоки, сравнение примеров, поиск значений'],
  ['Интерпретация', 'декларативная модель с версиями'],
  ['Проверка', 'корпус, нарушения, контрпримеры'],
  ['Передача', 'отчёт, машиночитаемая спецификация, проект'],
] as const;

export function ProjectsPage() {
  const { data, isLoading, error, refetch } = useQuery(projectQueries.list());

  return (
    <Page>
      <section className="brand-gradient relative mb-8 overflow-hidden rounded-xl border border-line px-8 py-8">
        <p className="text-[11px] font-semibold tracking-[0.18em] text-glow uppercase">Лаборатория протоколов</p>
        <h1 className="mt-2 max-w-2xl text-2xl font-bold tracking-tight text-white">
          Исследование и описание недокументированных бинарных протоколов поверх TCP
        </h1>
        <p className="mt-2 max-w-2xl text-[13px] text-white/75">
          Каждое утверждение связано с исходными байтами, проверяется на корпусе и воспроизводится после передачи проекта другому инженеру.
        </p>
        <ol className="mt-6 grid max-w-4xl grid-cols-2 gap-2 md:grid-cols-5">
          {STEPS.map(([title, text], i) => (
            <li key={title} className="rounded-lg border border-white/15 bg-black/25 px-3 py-2 backdrop-blur-sm">
              <p className="text-[11px] text-white/60">{i + 1}</p>
              <p className="text-[13px] font-semibold text-white">{title}</p>
              <p className="text-[11.5px] text-white/65">{text}</p>
            </li>
          ))}
        </ol>
      </section>

      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-base font-semibold">Проекты</h2>
        <div className="flex gap-2">
          <ImportProjectButton />
          <CreateProjectButton />
        </div>
      </div>

      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && (
        <EmptyState icon={<FolderOpen size={20} />} title="Проектов нет" description="Создайте проект и загрузите первую запись трафика." />
      )}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {data?.items.map((p) => (
          <Link key={p.id} to={routes.project(p.id)}>
            <Card className="h-full px-4 py-4 transition-colors hover:border-line-strong hover:bg-surface-2">
              <div className="flex items-start justify-between gap-2">
                <h3 className="font-semibold">{p.name}</h3>
                <span className="font-mono text-[11px] text-subtle">{p.id}</span>
              </div>
              <p className="mt-1 line-clamp-2 min-h-[2.6em] text-[12.5px] text-muted">{p.description || 'Без описания'}</p>
              <div className="mt-3 grid grid-cols-4 gap-2 text-center">
                {(
                  [
                    ['записи', p.stats.captures],
                    ['сессии', p.stats.sessions],
                    ['гипотезы', p.stats.hypotheses],
                    ['проверки', p.stats.runs],
                  ] as const
                ).map(([label, value]) => (
                  <div key={label} className="rounded-md bg-bg py-1.5">
                    <p className="font-mono text-[15px] font-semibold">{value}</p>
                    <p className="text-[10.5px] text-subtle">{label}</p>
                  </div>
                ))}
              </div>
              <p className="mt-3 text-[11px] text-subtle">Изменён {formatDateTime(p.updatedAt)}</p>
            </Card>
          </Link>
        ))}
      </div>
    </Page>
  );
}
