import type { RunSummary } from '@/entities/verification';
import { Status, Table, Tag, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export function RuleStatsTable({ summary }: { summary: RunSummary }) {
  return (
    <Table>
      <THead>
        <tr>
          <Th>Правило</Th>
          <Th className="text-right">Сообщ.</Th>
          <Th className="text-right">Наруш.</Th>
          <Th />
        </tr>
      </THead>
      <TBody>
        {summary.byRule.map((r) => (
          <Tr key={r.ruleId}>
            <Td className="font-mono text-xs">{r.ruleId}</Td>
            <Td className="text-right font-mono">{r.matched}</Td>
            <Td className="text-right font-mono">{r.violations || ''}</Td>
            <Td>
              {r.matched === 0 && <Tag>не применялось</Tag>}
              {r.violations > 0 && <Status tone="danger">есть контрпримеры</Status>}
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
