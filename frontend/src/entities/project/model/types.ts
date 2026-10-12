import type { ISODateTime } from '@/shared/api';

/** How to resolve retransmitted bytes that differ from the original ones. */
export type OverlapPolicy = 'first' | 'last';
/** TCP checksum errors are common with checksum offloading on the capturing host. */
export type ChecksumPolicy = 'ignore' | 'warn';

export interface ProjectSettings {
  overlapPolicy: OverlapPolicy;
  checksumPolicy: ChecksumPolicy;
  /** Server ports of the device under research; used to mark target sessions. */
  targetPorts: number[];
}

export interface ProjectStats {
  captures: number;
  sessions: number;
  actions: number;
  observations: number;
  hypotheses: number;
  interpretations: number;
  runs: number;
}

export interface Project {
  id: string;
  name: string;
  description: string;
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
  settings: ProjectSettings;
  stats: ProjectStats;
}

export interface CreateProjectInput {
  name: string;
  description?: string;
  targetPorts?: number[];
}
