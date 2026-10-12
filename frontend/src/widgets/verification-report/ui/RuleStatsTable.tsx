import type { RunSummary } from '@/entities/verification';
import { Badge, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

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
            <Td className="font-mono text-[12px]">{r.ruleId}</Td>
            <Td className="text-right font-mono">{r.matched}</Td>
            <Td className="text-right font-mono">{r.violations || ''}</Td>
            <Td>
              {r.matched === 0 && <Badge tone="info">не применялось</Badge>}
              {r.violations > 0 && <Badge tone="danger">есть контрпримеры</Badge>}
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
