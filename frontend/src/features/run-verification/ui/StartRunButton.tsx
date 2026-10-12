import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Play } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { corpusQueries, describeCorpusFilter } from '@/entities/corpus';
import { interpretationQueries } from '@/entities/interpretation';
import { jobKeys } from '@/entities/job';
import { projectKeys } from '@/entities/project';
import { verificationApi } from '@/entities/verification';
import { Button, type ButtonProps, Field, Modal, Select, toast } from '@/shared/ui';

export interface StartRunButtonProps {
  projectId: string;
  defaultVersionId?: string;
  defaultCorpusId?: string;
  label?: string;
  size?: ButtonProps['size'];
  variant?: ButtonProps['variant'];
}

/** Applies an interpretation version to a corpus as a background job. */
export function StartRunButton({
  projectId,
  defaultVersionId,
  defaultCorpusId,
  label = 'Запустить проверку',
  size = 'md',
  variant = 'primary',
}: StartRunButtonProps) {
  const [open, setOpen] = useState(false);
  const [interpretationId, setInterpretationId] = useState('');
  const [versionId, setVersionId] = useState(defaultVersionId ?? '');
  const [corpusId, setCorpusId] = useState(defaultCorpusId ?? '');
  const qc = useQueryClient();

  const interpretations = useQuery({ ...interpretationQueries.list(projectId), enabled: open });
  const activeInterpretation = interpretationId || interpretations.data?.items[0]?.id || '';
  const versions = useQuery({ ...interpretationQueries.versions(activeInterpretation), enabled: open && !!activeInterpretation });
  const corpora = useQuery({ ...corpusQueries.list(projectId), enabled: open });

  const effectiveVersion = versionId || versions.data?.items[0]?.id || '';
  const effectiveCorpus = corpusId || corpora.data?.items[0]?.id || '';
  const corpus = corpora.data?.items.find((c) => c.id === effectiveCorpus);

  const mutation = useMutation({
    mutationFn: () => verificationApi.start(projectId, { versionId: effectiveVersion, corpusId: effectiveCorpus }),
    onSuccess: ({ run }) => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      qc.invalidateQueries({ queryKey: jobKeys.all });
      toast.info(`Проверка ${run.id} запущена`);
      setOpen(false);
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <>
      <Button size={size} variant={variant} icon={<Play size={13} />} onClick={() => setOpen(true)}>
        {label}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Проверка интерпретации"
        description="Интерпретация применяется ко всем сессиям корпуса. В сводку попадают нарушения, неполные и неоднозначные данные, неизвестные участки и неприменимые правила."
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="start-run" loading={mutation.isPending} disabled={!effectiveVersion || !effectiveCorpus}>
              Запустить
            </Button>
          </>
        }
      >
        <form id="start-run" onSubmit={submit} className="flex flex-col gap-4">
          <div className="grid grid-cols-[1fr_140px] gap-3">
            <Field label="Интерпретация">
              <Select
                value={activeInterpretation}
                onChange={(e) => {
                  setInterpretationId(e.target.value);
                  setVersionId('');
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
              <Select value={effectiveVersion} onChange={(e) => setVersionId(e.target.value)}>
                {versions.data?.items.map((v) => (
                  <option key={v.id} value={v.id}>
                    v{v.version}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
          <Field label="Корпус" hint={corpus ? `${corpus.sessionIds.length} сессий · ${describeCorpusFilter(corpus.filter).join(' · ')}` : undefined}>
            <Select value={effectiveCorpus} onChange={(e) => setCorpusId(e.target.value)}>
              {corpora.data?.items.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
          </Field>
        </form>
      </Modal>
    </>
  );
}
