import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { interpretationQueries } from '@/entities/interpretation';
import { jobKeys } from '@/entities/job';
import { projectKeys } from '@/entities/project';
import { reportApi } from '@/entities/report';
import { verificationQueries } from '@/entities/verification';
import { Button, Checkbox, Field, Input, Modal, Select, Status, toast } from '@/shared/ui';

export function GenerateReportButton({ projectId }: { projectId: string }) {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState('');
  const [interpretationId, setInterpretationId] = useState('');
  const [versionId, setVersionId] = useState('');
  const [runIds, setRunIds] = useState<string[] | null>(null);
  const qc = useQueryClient();

  const interpretations = useQuery({ ...interpretationQueries.list(projectId), enabled: open });
  const interp = interpretations.data?.items.find((i) => i.id === interpretationId) ?? interpretations.data?.items[0];
  const versions = useQuery({ ...interpretationQueries.versions(interp?.id ?? ''), enabled: open && !!interp });
  const runs = useQuery({ ...verificationQueries.runs(projectId), enabled: open });

  const effectiveVersion = versionId || versions.data?.items[0]?.id || '';
  const versionRuns = (runs.data?.items ?? []).filter((r) => r.versionId === effectiveVersion && r.status === 'succeeded');
  const selectedRuns = runIds ?? versionRuns.map((r) => r.id);

  const mutation = useMutation({
    mutationFn: () => reportApi.create(projectId, { title, versionId: effectiveVersion, runIds: selectedRuns }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      qc.invalidateQueries({ queryKey: jobKeys.all });
      toast.info('Отчёт формируется');
      setOpen(false);
      setRunIds(null);
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        Сформировать отчёт
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Отчёт об исследовании"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="gen-report" loading={mutation.isPending} disabled={!effectiveVersion}>
              Сформировать
            </Button>
          </>
        }
      >
        <form id="gen-report" onSubmit={submit} className="flex flex-col gap-4">
          <Field label="Заголовок">
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={interp ? `${interp.name} — итоговый отчёт` : ''} />
          </Field>
          <div className="grid grid-cols-[1fr_140px] gap-3">
            <Field label="Интерпретация">
              <Select
                value={interp?.id ?? ''}
                onChange={(e) => {
                  setInterpretationId(e.target.value);
                  setVersionId('');
                  setRunIds(null);
                }}
              >
                {interpretations.data?.items.map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.name}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Версия">
              <Select
                value={effectiveVersion}
                onChange={(e) => {
                  setVersionId(e.target.value);
                  setRunIds(null);
                }}
              >
                {versions.data?.items.map((v) => (
                  <option key={v.id} value={v.id}>
                    v{v.version}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
          <div>
            <p className="mb-2 text-xs text-muted">Проверки, на которые опирается отчёт</p>
            {versionRuns.length === 0 && <p className="text-xs text-warn">Для этой версии нет завершённых проверок — выводы будут без подтверждения.</p>}
            <div className="flex flex-col gap-1.5">
              {versionRuns.map((r) => (
                <div key={r.id} className="flex items-center gap-2">
                  <Checkbox
                    label={`${r.id} — «${r.corpusName}»`}
                    checked={selectedRuns.includes(r.id)}
                    onChange={() =>
                      setRunIds(selectedRuns.includes(r.id) ? selectedRuns.filter((x) => x !== r.id) : [...selectedRuns, r.id])
                    }
                  />
                  {r.summary && r.summary.violations > 0 && <Status tone="danger" className="text-xs">нарушений: {r.summary.violations}</Status>}
                </div>
              ))}
            </div>
          </div>
        </form>
      </Modal>
    </>
  );
}
