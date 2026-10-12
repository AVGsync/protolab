import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { corpusQueries, describeCorpusFilter } from '@/entities/corpus';
import { interpretationQueries } from '@/entities/interpretation';
import { jobKeys } from '@/entities/job';
import { projectKeys } from '@/entities/project';
import { verificationApi } from '@/entities/verification';
import { Button, type ButtonProps, Field, Modal, Select, toast } from '@/shared/ui';

export interface StartRunDialogProps {
  projectId: string;
  open: boolean;
  onClose: () => void;
  defaultVersionId?: string;
  defaultCorpusId?: string;
}

/** Applies an interpretation version to a corpus as a background job. */
export function StartRunDialog({ projectId, open, onClose, defaultVersionId, defaultCorpusId }: StartRunDialogProps) {
  const [interpretationId, setInterpretationId] = useState('');
  const [versionId, setVersionId] = useState('');
  const [corpusId, setCorpusId] = useState('');
  const qc = useQueryClient();

  // The version the caller is looking at decides which interpretation the dialog starts with.
  const preset = useQuery({ ...interpretationQueries.version(defaultVersionId ?? ''), enabled: open && !!defaultVersionId });
  const interpretations = useQuery({ ...interpretationQueries.list(projectId), enabled: open });
  const activeInterpretation = interpretationId || preset.data?.interpretationId || interpretations.data?.items[0]?.id || '';
  const versions = useQuery({ ...interpretationQueries.versions(activeInterpretation), enabled: open && !!activeInterpretation });
  const corpora = useQuery({ ...corpusQueries.list(projectId), enabled: open });

  const presetApplies = !interpretationId || interpretationId === preset.data?.interpretationId;
  const effectiveVersion = versionId || (presetApplies ? defaultVersionId : undefined) || versions.data?.items[0]?.id || '';
  const effectiveCorpus = corpusId || defaultCorpusId || corpora.data?.items[0]?.id || '';
  const corpus = corpora.data?.items.find((c) => c.id === effectiveCorpus);

  const mutation = useMutation({
    mutationFn: () => verificationApi.start(projectId, { versionId: effectiveVersion, corpusId: effectiveCorpus }),
    onSuccess: ({ run }) => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      qc.invalidateQueries({ queryKey: jobKeys.all });
      toast.info(`Проверка ${run.id} запущена`);
      onClose();
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Проверка интерпретации"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
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
  );
}

export interface StartRunButtonProps extends Omit<StartRunDialogProps, 'open' | 'onClose'> {
  label?: string;
  size?: ButtonProps['size'];
  variant?: ButtonProps['variant'];
}

export function StartRunButton({ label = 'Запустить проверку', size = 'md', variant = 'primary', ...dialog }: StartRunButtonProps) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size={size} variant={variant} onClick={() => setOpen(true)}>
        {label}
      </Button>
      <StartRunDialog {...dialog} open={open} onClose={() => setOpen(false)} />
    </>
  );
}
