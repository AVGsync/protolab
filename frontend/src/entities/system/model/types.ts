/** Build info, declared protective limits and runtime resource usage of the backend. */
export interface SystemInfo {
  version: string;
  commit: string;
  buildDate: string;
  engineVersion: string;
  limits: {
    maxCaptureBytes: number;
    maxPackets: number;
    maxSessions: number;
    maxMessages: number;
    maxMessageBytes: number;
    /** One reassembled direction of a TCP session. */
    maxStreamBytes: number;
    /** All reassembled directions of one capture together. */
    maxTotalStreamBytes: number;
  };
  runtime: {
    os: string;
    arch: string;
    cpus: number;
    goVersion: string;
    memAllocBytes: number;
    memSysBytes: number;
    uptimeSec: number;
  };
  storage: {
    dataDir: string;
    usedBytes: number;
  };
}
