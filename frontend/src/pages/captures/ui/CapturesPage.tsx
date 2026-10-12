import { useQuery } from '@tanstack/react-query';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { Fragment, useState } from 'react';
import { Link } from 'react-router';
import { CaptureStatusBadge, captureQueries, DiagnosticList, IntegrityBadge } from '@/entities/capture';
import { CaptureDropzone, UploadCaptureButton } from '@/features/upload-capture';
import { routes } from '@/shared/config';
import { formatBytes, formatDateTime, formatNumber, formatTimestamp, shortHash, useRequiredParams } from '@/shared/lib';
import { Badge, Card, CopyButton, ErrorState, Loader, Page, PageHeader, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export function CapturesPage() {
  const { projectId } = useRequiredParams('projectId');
  const { data, isLoading, error, refetch } = useQuery(captureQueries.list(projectId));
  const [expanded, setExpanded] = useState<string | null>(null);

  return (
    <Page>
      <PageHeader
        title="Записи трафика"
        description="Исходные PCAP/PCAPNG хранятся без изменений. Все производные результаты ссылаются на запись по SHA-256 и номерам пакетов."
        actions={data && data.items.length > 0 && <UploadCaptureButton projectId={projectId} />}
      />
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && <CaptureDropzone projectId={projectId} />}
      {data && data.items.length > 0 && (
        <Card>
          <Table>
            <THead>
              <tr>
                <Th className="w-8" />
                <Th>Файл</Th>
                <Th>SHA-256</Th>
                <Th className="text-right">Размер</Th>
                <Th className="text-right">Пакеты</Th>
                <Th className="text-right">Сессии</Th>
                <Th>Интервал (UTC)</Th>
                <Th>Диагностика</Th>
                <Th>Статус</Th>
              </tr>
            </THead>
            <TBody>
              {data.items.map((c) => {
                const warnings = c.diagnostics.filter((d) => d.level !== 'info').length;
                const open = expanded === c.id;
                return (
                  <Fragment key={c.id}>
                    <Tr onClick={() => setExpanded(open ? null : c.id)}>
                      <Td className="text-subtle">{open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}</Td>
                      <Td>
                        <p className="font-medium">{c.fileName}</p>
                        <p className="text-[11px] text-subtle">
                          {c.format.toUpperCase()} · {c.linkType} · snaplen {c.snapLen} · импорт {formatDateTime(c.importedAt)}
                        </p>
                      </Td>
                      <Td onClick={(e) => e.stopPropagation()}>
                        <span className="inline-flex items-center gap-1 font-mono text-[11.5px] text-muted" title={c.sha256}>
                          {shortHash(c.sha256, 16)}…
                          <CopyButton value={c.sha256} />
                        </span>
                        <div className="mt-0.5">
                          <IntegrityBadge integrity={c.integrity} />
                        </div>
                      </Td>
                      <Td className="text-right font-mono text-[12px] whitespace-nowrap">{formatBytes(c.sizeBytes)}</Td>
                      <Td className="text-right font-mono text-[12px]">{formatNumber(c.packetCount)}</Td>
                      <Td className="text-right font-mono text-[12px] whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                        <Link to={routes.sessions(projectId, { captureId: c.id })} className="text-info hover:underline">
                          {c.sessionCount}
                        </Link>
                        <span className="text-subtle"> / {c.targetSessionCount} цел.</span>
                      </Td>
                      <Td className="font-mono text-[11.5px] whitespace-nowrap text-muted">
                        {c.firstPacketAt ? `${c.firstPacketAt.slice(0, 10)} ${formatTimestamp(c.firstPacketAt, 0)}–${formatTimestamp(c.lastPacketAt!, 0)}` : '—'}
                      </Td>
                      <Td>
                        {warnings > 0 ? <Badge tone="warning">{warnings} предупр.</Badge> : <Badge tone="success">чисто</Badge>}
                      </Td>
                      <Td>
                        <CaptureStatusBadge status={c.status} />
                      </Td>
                    </Tr>
                    {open && (
                      <tr className="border-b border-line bg-bg/60">
                        <td />
                        <td colSpan={8} className="px-3 py-3">
                          <DiagnosticList items={c.diagnostics} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </TBody>
          </Table>
        </Card>
      )}
    </Page>
  );
}
