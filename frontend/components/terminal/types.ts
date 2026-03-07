export interface SessionInfo {
  id: string;
  name: string;
}

export interface MetricsSnapshot {
  timestamp: string;
  cpu: {
    cores: number;
    usage_percent: number;
    per_core: number[];
    available: boolean;
  };
  memory: {
    total_bytes: number;
    used_bytes: number;
    free_bytes: number;
    available_bytes: number;
    cached_bytes: number;
    swap_total_bytes: number;
    swap_used_bytes: number;
    available: boolean;
  };
  process: {
    pid: number;
    cpu_percent: number;
    memory_bytes: number;
    goroutines: number;
    available: boolean;
  };
  top_cpu: Array<{
    pid: number;
    name: string;
    cpu_percent: number;
    memory_bytes: number;
  }>;
  top_memory: Array<{
    pid: number;
    name: string;
    cpu_percent: number;
    memory_bytes: number;
  }>;
  gpu: {
    available: boolean;
    note: string;
  };
}
