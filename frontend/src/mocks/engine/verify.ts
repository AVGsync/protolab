import type { InterpretationSpec } from '@/entities/interpretation';
import type { Message } from '@/entities/message';
import type { Finding, FindingKind, RunSummary } from '@/entities/verification';
import type { Direction, SourceRef } from '@/shared/api';
import { hexByte } from '@/shared/lib';
import { flatten, parseSessionDetailed, type SessionInput } from './interpreter';

const MAX_EXAMPLES = 5;

interface Bucket {
  kind: FindingKind;
  severity: Finding['severity'];
  ruleId?: string;
  fieldPath?: string;
  message: string;
  occurrences: number;
  examples: SourceRef[];
}

export interface VerificationResult {
  summary: RunSummary;
  findings: Omit<Finding, 'id' | 'runId'>[];
}

function ref(input: SessionInput, dir: Direction, offset: number, length: number): SourceRef {
  const segments = input.streams[dir].segments;
  return {
    captureId: input.session.captureId,
    captureSha256: input.session.capture.sha256,
    sessionId: input.session.id,
    direction: dir,
    offset,
    length: Math.max(1, length),
    packetNumbers: segments
      .filter((s) => s.offset < offset + length && offset < s.offset + s.length)
      .map((s) => s.packetNumber),
  };
}

/**
 * Applies an interpretation to every session of a corpus. The summary covers
 * the whole selection — failures, partial data and unknown bytes included.
 */
export function verify(spec: InterpretationSpec, inputs: SessionInput[]): VerificationResult {
  const buckets = new Map<string, Bucket>();
  const add = (key: string, init: Omit<Bucket, 'occurrences' | 'examples'>, example?: SourceRef) => {
    let b = buckets.get(key);
    if (!b) {
      b = { ...init, occurrences: 0, examples: [] };
      buckets.set(key, b);
    }
    b.occurrences++;
    if (example && b.examples.length < MAX_EXAMPLES) b.examples.push(example);
  };

  const summary: RunSummary = {
    sessions: inputs.length,
    sessionsWithErrors: 0,
    messages: 0,
    ok: 0,
    violations: 0,
    partial: 0,
    ambiguous: 0,
    unknownType: 0,
    unknownBytes: 0,
    totalBytes: 0,
    inapplicableRules: [],
    byRule: spec.messages.map((r) => ({ ruleId: r.id, matched: 0, violations: 0 })),
  };
  const ruleStat = new Map(summary.byRule.map((r) => [r.ruleId, r]));

  for (const input of inputs) {
    let messages: Message[];
    let unframed: { direction: Direction; offset: number; length: number }[];
    try {
      ({ messages, unframed } = parseSessionDetailed(spec, input));
    } catch (err) {
      // An error in one session must not block the rest of the corpus.
      summary.sessionsWithErrors++;
      add(`engine:${input.session.id}`, {
        kind: 'capture_defect',
        severity: 'error',
        message: `Ошибка обработки сессии: ${err instanceof Error ? err.message : String(err)}`,
      });
      continue;
    }

    let sessionHasErrors = false;
    for (const dir of ['c2s', 's2c'] as const) {
      const s = input.streams[dir];
      for (const g of s.gaps) {
        sessionHasErrors = true;
        add(
          `gap:${g.reason}`,
          {
            kind: 'capture_defect',
            severity: 'warning',
            message: g.reason === 'truncated' ? 'Пакеты усечены при захвате — хвосты неизвестны' : 'Потерянные сегменты — участки потока неизвестны',
          },
          ref(input, dir, g.offset, g.length),
        );
      }
    }
    if (input.session.flags.overlapConflicts > 0) {
      sessionHasErrors = true;
      add('conflict', { kind: 'capture_defect', severity: 'warning', message: 'Повторные передачи с разными данными' }, ref(input, 'c2s', 0, 1));
    }

    for (const u of unframed) {
      summary.unknownBytes += u.length;
      add(
        'unframed',
        { kind: 'unknown_region', severity: 'warning', message: 'Байты вне сообщений: синхронизация кадров потеряна (начало не захвачено или пропуск)' },
        ref(input, u.direction, u.offset, u.length),
      );
    }

    for (const m of messages) {
      summary.messages++;
      summary.totalBytes += m.length;
      const msgRef = ref(input, m.direction, m.offset, m.length);
      const stat = m.ruleId ? ruleStat.get(m.ruleId) : undefined;
      if (stat) stat.matched++;

      switch (m.status) {
        case 'ok':
          summary.ok++;
          break;
        case 'violation':
          summary.violations++;
          if (stat) stat.violations++;
          break;
        case 'partial':
          summary.partial++;
          add(`partial:${m.ruleId}`, { kind: 'partial', severity: 'warning', ruleId: m.ruleId ?? undefined, message: m.issues[0] ?? 'Сообщение неполное' }, msgRef);
          break;
        case 'ambiguous':
          summary.ambiguous++;
          add(`ambiguous:${m.candidateRuleIds?.join(',')}`, { kind: 'ambiguous', severity: 'warning', message: `Подходят несколько правил: ${m.candidateRuleIds?.join(', ')}` }, msgRef);
          break;
        case 'unknown_type': {
          summary.unknownType++;
          const data = atob(m.data);
          const head = [...data.slice(0, 2)].map((c) => hexByte(c.charCodeAt(0))).join(' ');
          add(
            `unknown_type:${m.direction}:${head}`,
            { kind: 'unknown_type', severity: 'info', message: `${m.direction === 'c2s' ? 'C→S' : 'S→C'}: сообщения «${head} …» не описаны ни одним правилом` },
            msgRef,
          );
          break;
        }
      }
      if (m.status !== 'ok') sessionHasErrors ||= m.status !== 'unknown_type';

      for (const f of flatten(m.fields)) {
        if (f.status !== 'violation') continue;
        const genericPath = f.path.replace(/\[\d+\]/g, '[]');
        const kind = f.issue?.includes('≠') ? 'assertion' : f.issue?.split(' ')[0];
        add(
          `violation:${m.ruleId}:${genericPath}:${kind}`,
          { kind: 'violation', severity: 'error', ruleId: m.ruleId ?? undefined, fieldPath: genericPath, message: f.issue ?? 'Нарушение правила' },
          ref(input, m.direction, m.offset + f.offset, f.length),
        );
      }

      if (m.ruleId && m.status !== 'partial') {
        for (const u of m.unknown) {
          summary.unknownBytes += u.length;
          const fromEnd = u.offset + u.length === m.length;
          const where = fromEnd && u.length === 1 ? 'последний байт' : `байты ${u.offset}–${u.offset + u.length - 1}`;
          add(
            `unknown_region:${m.ruleId}:${fromEnd ? `end-${u.length}` : u.offset}`,
            { kind: 'unknown_region', severity: 'info', ruleId: m.ruleId, message: `${m.ruleId}: ${where} не описаны полями` },
            ref(input, m.direction, m.offset + u.offset, u.length),
          );
        }
      } else if (!m.ruleId) {
        summary.unknownBytes += m.length;
      }
    }
    if (sessionHasErrors) summary.sessionsWithErrors++;
  }

  summary.inapplicableRules = summary.byRule.filter((r) => r.matched === 0).map((r) => r.ruleId);
  for (const ruleId of summary.inapplicableRules) {
    add(`inapplicable:${ruleId}`, {
      kind: 'inapplicable_rule',
      severity: 'info',
      ruleId,
      message: `Правило «${ruleId}» не сработало ни на одном сообщении корпуса — его корректность не проверена`,
    });
  }

  const order: FindingKind[] = ['violation', 'partial', 'ambiguous', 'capture_defect', 'unknown_type', 'unknown_region', 'inapplicable_rule'];
  const findings = [...buckets.values()].sort((a, b) => order.indexOf(a.kind) - order.indexOf(b.kind) || b.occurrences - a.occurrences);
  return { summary, findings };
}
