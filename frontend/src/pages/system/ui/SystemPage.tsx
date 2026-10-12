import { useQuery } from '@tanstack/react-query';
import { systemQueries } from '@/entities/system';
import { env } from '@/shared/config';
import { formatBytes, formatDuration, formatNumber } from '@/shared/lib';
import { Card, CardBody, CardHeader, ErrorState, KeyValue, Loader, Mono, Page, PageHeader, Stat } from '@/shared/ui';

export function SystemPage() {
  const { data, isLoading, error, refetch } = useQuery(systemQueries.info());

  return (
    <Page>
      <PageHeader
        title="Система и ресурсы"
        description="Заявленные защитные пределы и текущее потребление ресурсов backend. Превышение пределов обрабатывается явно, без скрытого обрезания данных."
      />
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && (
        <div className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            <Stat label="Память (heap)" value={formatBytes(data.runtime.memAllocBytes)} hint={`из ОС: ${formatBytes(data.runtime.memSysBytes)}`} />
            <Stat label="CPU" value={data.runtime.cpus} hint={`${data.runtime.os}/${data.runtime.arch}`} />
            <Stat label="Хранилище" value={formatBytes(data.storage.usedBytes)} hint={data.storage.dataDir} />
            <Stat label="Аптайм" value={formatDuration(data.runtime.uptimeSec * 1000)} />
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader title="Защитные пределы" description="Контрольная нагрузка: 100 МиБ, 250 000 пакетов, 1000 соединений, 100 000 сообщений" />
              <CardBody>
                <KeyValue
                  items={[
                    ['Размер записи', formatBytes(data.limits.maxCaptureBytes)],
                    ['Пакетов', formatNumber(data.limits.maxPackets)],
                    ['TCP-сессий', formatNumber(data.limits.maxSessions)],
                    ['Сообщений', formatNumber(data.limits.maxMessages)],
                    ['Размер сообщения', formatBytes(data.limits.maxMessageBytes)],
                  ]}
                />
              </CardBody>
            </Card>
            <Card>
              <CardHeader title="Сборка" />
              <CardBody>
                <KeyValue
                  items={[
                    ['Версия', <Mono key="v">{data.version}</Mono>],
                    ['Коммит', <Mono key="c">{data.commit}</Mono>],
                    ['Дата сборки', data.buildDate],
                    ['Движок', <Mono key="e">{data.engineVersion}</Mono>],
                    ['Go', <Mono key="g">{data.runtime.goVersion}</Mono>],
                    ['API', <Mono key="a">{env.apiBaseUrl}{env.useMocks ? ' (mock)' : ''}</Mono>],
                  ]}
                />
              </CardBody>
            </Card>
          </div>
        </div>
      )}
    </Page>
  );
}
