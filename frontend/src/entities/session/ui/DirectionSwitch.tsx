import type { Direction } from '@/shared/api';
import { Segmented } from '@/shared/ui';
import { DIRECTION_LABEL } from '../model/labels';
import { DirectionLabel } from './SessionMarks';

export interface DirectionSwitchProps {
  value: Direction;
  onChange: (direction: Direction) => void;
  /** Stream length per direction: the switch doubles as a size readout. */
  bytes: { c2s: number; s2c: number };
}

/** Segmented control for the direction of the stream; each side keeps its channel colour. */
export function DirectionSwitch({ value, onChange, bytes }: DirectionSwitchProps) {
  return (
    <Segmented
      aria-label="Направление потока"
      value={value}
      onChange={onChange}
      items={(['c2s', 's2c'] as const).map((d) => ({
        id: d,
        title: DIRECTION_LABEL[d],
        label: (
          <>
            <DirectionLabel direction={d} />
            <span className="font-mono text-2xs text-muted">{bytes[d]} Б</span>
          </>
        ),
      }))}
    />
  );
}
