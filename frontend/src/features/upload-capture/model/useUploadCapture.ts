import { useMutation, useQueryClient } from '@tanstack/react-query';
import { captureApi } from '@/entities/capture';
import { jobKeys } from '@/entities/job';
import { projectKeys } from '@/entities/project';
import { toast } from '@/shared/ui';

export function useUploadCapture(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => captureApi.upload(projectId, file),
    onSuccess: ({ capture }) => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      qc.invalidateQueries({ queryKey: jobKeys.all });
      toast.info(`«${capture.fileName}» загружен, идёт индексация`);
    },
    onError: (err) => toast.fromError(err, 'Не удалось загрузить запись'),
  });
}
