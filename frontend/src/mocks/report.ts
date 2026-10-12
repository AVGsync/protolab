import type { Report, ReportArtifact } from '@/entities/report';
import { db, runDto } from './db';
import { fakeSha256 } from './fixtures/hash';

const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

function fileNames(report: Report) {
  const base = `${slug(report.interpretationName)}.v${report.version}`;
  return {
    report_md: `report.${base}.md`,
    interpretation: `interpretation.${base}.json`,
    results: `results.${base}.json`,
    examples: `examples.${base}.json`,
  } as const;
}

export function reportDto(report: Report): Report {
  if (report.status !== 'ready') return report;
  const names = fileNames(report);
  const artifacts: ReportArtifact[] = (Object.keys(names) as (keyof typeof names)[]).map((kind) => {
    const content = renderArtifact(report, names[kind]) ?? '';
    return {
      kind,
      fileName: names[kind],
      url: `/api/v1/reports/${report.id}/files/${names[kind]}`,
      sizeBytes: new TextEncoder().encode(content).length,
      sha256: fakeSha256(content),
    };
  });
  return {
    ...report,
    artifacts,
    reproduceCommand: [
      `protolab apply --spec ${names.interpretation} --input <capture.pcapng> --out results.json`,
      `protolab verify --spec ${names.interpretation} --examples ${names.examples}`,
    ].join('\n'),
  };
}

export function renderArtifact(report: Report, fileName: string): string | undefined {
  const names = fileNames(report);
  const version = db.versions.find((v) => v.id === report.versionId);
  if (!version) return undefined;
  const runs = report.runIds.map((id) => db.runs.find((r) => r.run.id === id)).filter((r) => r !== undefined);

  if (fileName === names.interpretation) {
    return JSON.stringify({ ...version.spec, $meta: { version: version.version, hash: version.hash, createdAt: version.createdAt } }, null, 2);
  }
  if (fileName === names.results) {
    return JSON.stringify(
      runs.map((r) => ({ run: runDto(r), findings: r.findings })),
      null,
      2,
    );
  }
  if (fileName === names.examples) {
    const corpusIds = new Set(runs.map((r) => r.run.corpusId));
    const sessions = db.corpora.filter((c) => corpusIds.has(c.id)).flatMap((c) => c.sessionIds);
    return JSON.stringify(
      {
        spec: names.interpretation,
        note: 'Входные сессии и ожидаемые сводки; protolab verify сравнивает результат применения интерпретации с ожидаемым.',
        inputs: [...new Set(sessions)].map((id) => {
          const s = db.sessions.get(id)!.session;
          return { sessionId: id, capture: s.capture.fileName, sha256: s.capture.sha256, tcpStream: s.index };
        }),
        expected: runs.map((r) => ({ corpus: r.run.corpusName, summary: r.run.summary })),
      },
      null,
      2,
    );
  }
  if (fileName === names.report_md) return renderMarkdown(report);
  return undefined;
}

function renderMarkdown(report: Report): string {
  const version = db.versions.find((v) => v.id === report.versionId)!;
  const hypotheses = db.hypotheses.filter((h) => h.projectId === report.projectId);
  const runs = report.runIds.map((id) => db.runs.find((r) => r.run.id === id)).filter((r) => r !== undefined);
  const lines: string[] = [
    `# ${report.title}`,
    '',
    `Интерпретация: **${report.interpretationName} v${version.version}** (\`${version.hash.slice(0, 16)}\`), сформировано ${report.createdAt}.`,
    '',
    '## Описание протокола',
    '',
  ];
  const f = version.spec.framing;
  if (f.kind === 'length_prefixed') {
    lines.push(`Кадрирование: длина u${f.lengthField.size * 8} ${f.lengthField.endian.toUpperCase()} по смещению ${f.lengthField.offset}, полная длина = значение + ${f.lengthAdjust}; синхронизация по \`${f.sync ?? '—'}\`.`, '');
  }
  for (const m of version.spec.messages) {
    lines.push(`### ${m.id}${m.title ? ` — ${m.title}` : ''}`, '', '| Поле | Смещение | Тип | Назначение | Статус |', '|---|---|---|---|---|');
    for (const field of m.fields) {
      lines.push(`| ${field.name} | ${field.offset} | ${field.type}${field.endian ? ` ${field.endian}` : ''} | ${field.meaning ?? '—'} | ${field.status ?? 'structural'} |`);
    }
    lines.push('');
  }
  lines.push('## Проверки', '');
  for (const r of runs) {
    const s = r.run.summary!;
    lines.push(
      `- **${r.run.id}** — корпус «${r.run.corpusName}»: ${s.sessions} сессий, ${s.messages} сообщений; разобрано ${s.ok}, нарушений ${s.violations}, неполных ${s.partial}, неизвестных типов ${s.unknownType}, неописанных байтов ${s.unknownBytes}.`,
    );
    if (s.inapplicableRules.length) lines.push(`  Неприменимые правила: ${s.inapplicableRules.join(', ')}.`);
  }
  lines.push('', '> Совпадение на выбранных примерах не доказывает назначение полей; см. статусы гипотез и контрпримеры.', '', '## Гипотезы', '');
  for (const h of hypotheses) {
    lines.push(`- [${h.status}] **${h.title}** — ${h.statement}`);
    for (const c of h.counterexamples) lines.push(`  - контрпример: ${c.note} (${c.source.sessionId}/${c.source.direction} @${c.source.offset})`);
  }
  lines.push('', '## Открытые вопросы', '');
  for (const h of hypotheses.filter((x) => x.status === 'proposed' || x.status === 'testing')) lines.push(`- ${h.title}`);
  return lines.join('\n');
}
