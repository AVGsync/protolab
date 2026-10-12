import { useNavigate, useParams } from 'react-router';
import { PROJECT_SECTIONS } from '@/shared/config';
import { Keycap } from '@/shared/ui';
import { useGlobalShortcuts } from '../model/useGlobalShortcuts';
import { useSoftKeysStore } from '../model/store';
import { HelpOverlay } from './HelpOverlay';

function SoftKey({ keyLabel, label, disabled, onPress }: { keyLabel: string; label: string; disabled?: boolean; onPress: () => void }) {
  return (
    <button
      type="button"
      onClick={onPress}
      disabled={disabled}
      className="inline-flex items-center gap-1.5 whitespace-nowrap hover:text-fg disabled:opacity-45 disabled:hover:text-muted"
    >
      <Keycap>{keyLabel}</Keycap>
      {label}
    </button>
  );
}

/**
 * Global soft-key footer (mirrors the TUI): keycaps for the actions of the current screen,
 * section hotkeys when the screen has none, and a slot on the right for a live readout.
 */
export function SoftKeys() {
  useGlobalShortcuts();
  const { projectId } = useParams();
  const navigate = useNavigate();
  const groups = useSoftKeysStore((s) => s.groups);
  const helpOpen = useSoftKeysStore((s) => s.helpOpen);
  const setHelpOpen = useSoftKeysStore((s) => s.setHelpOpen);
  const setReadoutEl = useSoftKeysStore((s) => s.setReadoutEl);
  const contextKeys = groups.flatMap((g) => g.keys);

  return (
    <footer className="flex h-7 shrink-0 items-center gap-4 border-t border-line bg-surface px-4 text-xs text-muted">
      {contextKeys.length > 0
        ? contextKeys.map((k) => <SoftKey key={k.key} keyLabel={k.key} label={k.label} disabled={k.disabled} onPress={k.onPress} />)
        : projectId &&
          PROJECT_SECTIONS.map((s) => (
            <SoftKey key={s.id} keyLabel={s.key} label={s.label.toLowerCase()} onPress={() => navigate(s.to(projectId))} />
          ))}
      <SoftKey keyLabel="?" label="клавиши" onPress={() => setHelpOpen(true)} />
      <div ref={setReadoutEl} className="ml-auto flex min-w-0 items-center gap-4 font-mono text-fg" />
      <HelpOverlay open={helpOpen} onClose={() => setHelpOpen(false)} inProject={!!projectId} contextKeys={contextKeys} />
    </footer>
  );
}
