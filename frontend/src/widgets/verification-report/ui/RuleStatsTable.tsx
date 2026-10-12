import type { RunSummary } from '@/entities/verification';
import { Table, Tag, TBody, Td, Th, THead, Tr } from '@/shared/ui';

/** A violation count is the only coloured number: a rule with counterexamples is the thing to look at. */
export function RuleStatsTable({ summary }: { summary: RunSummary }) {
  return (
    <Table>
      <THead>
        <tr>
          <Th>Правило</Th>
          <Th className="text-right">Сообщ.</Th>
          <Th className="text-right" title="Нарушений правила: контрпримеры">
            Наруш.
          </Th>
          <Th />
        </tr>
      </THead>
      <TBody>
        {summary.byRule.map((r) => (
          <Tr key={r.ruleId}>
            <Td className="font-mono text-xs">{r.ruleId}</Td>
            <Td className="text-right font-mono text-xs">{r.matched}</Td>
            <Td className="text-right font-mono text-xs font-semibold text-danger">{r.violations || ''}</Td>
            <Td>{r.matched === 0 && <Tag>не применялось</Tag>}</Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
