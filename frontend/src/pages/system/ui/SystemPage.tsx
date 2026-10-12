import { useQuery } from '@tanstack/react-query';
import { systemQueries } from '@/entities/system';
import { env } from '@/shared/config';
import { formatBytes, formatDuration, formatNumber } from '@/shared/lib';
import { ErrorState, KeyValue, Loader, Mono, Page, PageHeader, Pane, PaneHeader, Readout } from '@/shared/ui';

export function SystemPage() {
  const { data, isLoading, error, refetch } = useQuery(systemQueries.info());

  return (
    <Page>
      <PageHeader title="Система" />
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && (
        <div className="flex flex-col gap-4">
          <Readout
            className="border-y border-line py-2"
            items={[
              { label: 'heap', value: formatBytes(data.runtime.memAllocBytes) },
              { label: 'из ОС', value: formatBytes(data.runtime.memSysBytes) },
              { label: 'CPU', value: `${data.runtime.cpus} (${data.runtime.os}/${data.runtime.arch})` },
              { label: 'хранилище', value: formatBytes(data.storage.usedBytes) },
              { label: 'аптайм', value: formatDuration(data.runtime.uptimeSec * 1000) },
            ]}
          />
          <div className="grid gap-4 md:grid-cols-2">
            <Pane>
              <PaneHeader title="Защитные пределы" />
              <div className="flex flex-col gap-3 p-3">
                <KeyValue
                  items={[
                    ['Размер записи', formatBytes(data.limits.maxCaptureBytes)],
                    ['Пакетов', formatNumber(data.limits.maxPackets)],
                    ['TCP-сессий', formatNumber(data.limits.maxSessions)],
                    ['Сообщений', formatNumber(data.limits.maxMessages)],
                    ['Размер сообщения', formatBytes(data.limits.maxMessageBytes)],
                  ]}
                />
                <p className="text-xs text-muted">Превышение предела обрабатывается явно, данные не обрезаются молча.</p>
              </div>
            </Pane>
            <Pane>
              <PaneHeader title="Сборка" />
              <div className="p-3">
                <KeyValue
                  items={[
                    ['Версия', <Mono key="v">{data.version}</Mono>],
                    ['Коммит', <Mono key="c">{data.commit}</Mono>],
                    ['Дата сборки', data.buildDate],
                    ['Движок', <Mono key="e">{data.engineVersion}</Mono>],
                    ['Go', <Mono key="g">{data.runtime.goVersion}</Mono>],
                    ['API', <Mono key="a">{env.apiBaseUrl}{env.useMocks ? ' (mock)' : ''}</Mono>],
                    ['Хранилище', <Mono key="d">{data.storage.dataDir}</Mono>],
                  ]}
                />
              </div>
            </Pane>
          </div>
        </div>
      )}
    </Page>
  );
}
