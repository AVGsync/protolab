import type { ISODateTime } from '@/shared/api';

export type ActionKind = 'read_params' | 'set_param' | 'get_measurement' | 'read_log' | 'other';

export interface ActionLink {
  sessionId: string;
  /** auto_time — matched by timestamp proximity; manual — set by the researcher. */
  method: 'auto_time' | 'manual';
  deltaMs?: number;
}

/** A known action performed in the vendor client (experiment condition). */
export interface ActionEntry {
  id: string;
  projectId: string;
  timestamp: ISODateTime;
  kind: ActionKind;
  label: string;
  /** Conditions of the experiment, e.g. {param: "setpoint", value: 21}. */
  params: Record<string, string | number>;
  /** Result observed in the client UI, e.g. {temperature: 22.4}. */
  observed?: Record<string, string | number>;
  result?: string;
  links: ActionLink[];
  source: 'import' | 'manual';
}

export interface CreateActionInput {
  timestamp: ISODateTime;
  kind: ActionKind;
  label: string;
  params: Record<string, string | number>;
  observed?: Record<string, string | number>;
  result?: string;
  sessionIds?: string[];
}

export interface ImportActionsResult {
  imported: number;
  linked: number;
  warnings: string[];
}
