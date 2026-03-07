"use client";

import { Cpu, MemoryStick, Monitor, X } from "lucide-react";

import type { MetricsSnapshot } from "./types";
import { formatBytes } from "./utils";

interface MetricsModalProps {
  open: "cpu" | "memory" | "gpu" | null;
  metrics: MetricsSnapshot | null;
  metricsPage: number;
  onClose: () => void;
  onPageChange: (next: number) => void;
}

export function MetricsModal({ open, metrics, metricsPage, onClose, onPageChange }: MetricsModalProps) {
  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="w-full max-w-2xl rounded-3xl border border-[var(--app-border)] bg-[var(--app-surface)] p-6 shadow-[0_30px_80px_rgba(0,0,0,0.35)]"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-2 text-[var(--app-text)]">
            {open === "cpu" && <Cpu className="h-4 w-4" />}
            {open === "memory" && <MemoryStick className="h-4 w-4" />}
            {open === "gpu" && <Monitor className="h-4 w-4" />}
            <span className="text-sm font-semibold">
              {open === "cpu" && "CPU Usage"}
              {open === "memory" && "Memory Usage"}
              {open === "gpu" && "GPU Usage"}
            </span>
          </div>
          <button
            type="button"
            className="rounded-md p-1 text-[var(--app-text-muted)] hover:text-[var(--app-text)]"
            onClick={onClose}
            aria-label="Close metrics"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="grid gap-4 text-sm">
          {open === "cpu" && (
            <div className="rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface-muted)]/70 p-4">
              <div className="mb-4 flex items-center justify-between">
                <div>
                  <div className="text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">Overall</div>
                  <div className="text-lg font-semibold text-[var(--app-text)]">
                    {metrics?.cpu?.available ? `${metrics.cpu.usage_percent.toFixed(1)}%` : "N/A"}
                  </div>
                  <div className="text-xs text-[var(--app-text-muted)]">Cores: {metrics?.cpu?.cores ?? "-"}</div>
                </div>
                <div className="h-12 w-12 rounded-full border border-[var(--app-border)] bg-[var(--app-surface)] p-1 shadow-inner">
                  <div
                    className="h-full w-full rounded-full bg-[var(--app-accent)]/30"
                    style={{
                      background: `conic-gradient(var(--app-accent) ${Math.min(100, metrics?.cpu?.usage_percent ?? 0)}%, transparent 0)`,
                    }}
                  />
                </div>
              </div>
              {metrics?.cpu?.available && metrics.cpu.per_core?.length ? (
                <div className="grid grid-cols-2 gap-2 text-xs">
                  {metrics.cpu.per_core.map((core, idx) => (
                    <div key={`core-${idx}`} className="rounded-lg border border-[var(--app-border)] bg-[var(--app-surface)] px-2 py-2">
                      <div className="flex items-center justify-between">
                        <span>Core {idx + 1}</span>
                        <span>{core.toFixed(0)}%</span>
                      </div>
                      <div className="mt-1 h-1.5 w-full rounded-full bg-[var(--app-border)]">
                        <div
                          className="h-1.5 rounded-full bg-[var(--app-accent)]"
                          style={{ width: `${Math.min(100, core)}%` }}
                        />
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="mt-3 text-xs text-[var(--app-text-muted)]">Per-core usage unavailable.</div>
              )}
              {metrics?.top_cpu?.length ? (
                <div className="mt-4 text-xs">
                  <div className="mb-2 flex items-center justify-between text-[var(--app-text-muted)]">
                    <span>Top CPU Processes</span>
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        className="rounded border border-[var(--app-border)] px-2 py-0.5"
                        onClick={() => onPageChange(Math.max(0, metricsPage - 1))}
                      >
                        Prev
                      </button>
                      <button
                        type="button"
                        className="rounded border border-[var(--app-border)] px-2 py-0.5"
                        onClick={() => onPageChange(metricsPage + 1)}
                      >
                        Next
                      </button>
                    </div>
                  </div>
                  <div className="space-y-2">
                    {metrics.top_cpu.map((proc) => (
                      <div key={`cpu-${proc.pid}`} className="flex items-center justify-between rounded-lg border border-[var(--app-border)] bg-[var(--app-surface)] px-2 py-2">
                        <span className="truncate">
                          {proc.name} <span className="text-[var(--app-text-muted)]">PID {proc.pid}</span>
                        </span>
                        <span className="text-[var(--app-text)]">{proc.cpu_percent.toFixed(1)}%</span>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}
            </div>
          )}

          {open === "memory" && (
            <div className="rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface-muted)]/70 p-4">
              <div className="mb-4 flex items-center justify-between">
                <div>
                  <div className="text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">Memory</div>
                  <div className="text-lg font-semibold text-[var(--app-text)]">
                    {metrics?.memory?.available
                      ? `${formatBytes(metrics.memory.used_bytes)} / ${formatBytes(metrics.memory.total_bytes)}`
                      : "N/A"}
                  </div>
                  <div className="text-xs text-[var(--app-text-muted)]">
                    Process: {metrics?.process?.memory_bytes ? formatBytes(metrics.process.memory_bytes) : "N/A"}
                  </div>
                </div>
                <div className="h-12 w-12 rounded-full border border-[var(--app-border)] bg-[var(--app-surface)] p-1 shadow-inner">
                  <div
                    className="h-full w-full rounded-full bg-[var(--app-accent)]/30"
                    style={{
                      background: `conic-gradient(var(--app-accent) ${metrics?.memory?.available && metrics.memory.total_bytes ? Math.min(100, (metrics.memory.used_bytes / metrics.memory.total_bytes) * 100) : 0}%, transparent 0)`,
                    }}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3 text-xs">
                <div className="rounded-lg border border-[var(--app-border)] bg-[var(--app-surface)] px-2 py-2">
                  <div className="text-[var(--app-text-muted)]">Free</div>
                  <div className="text-[var(--app-text)]">{formatBytes(metrics?.memory?.free_bytes ?? 0)}</div>
                </div>
                <div className="rounded-lg border border-[var(--app-border)] bg-[var(--app-surface)] px-2 py-2">
                  <div className="text-[var(--app-text-muted)]">Available</div>
                  <div className="text-[var(--app-text)]">{formatBytes(metrics?.memory?.available_bytes ?? 0)}</div>
                </div>
                <div className="rounded-lg border border-[var(--app-border)] bg-[var(--app-surface)] px-2 py-2">
                  <div className="text-[var(--app-text-muted)]">Cached</div>
                  <div className="text-[var(--app-text)]">{formatBytes(metrics?.memory?.cached_bytes ?? 0)}</div>
                </div>
                <div className="rounded-lg border border-[var(--app-border)] bg-[var(--app-surface)] px-2 py-2">
                  <div className="text-[var(--app-text-muted)]">Swap</div>
                  <div className="text-[var(--app-text)]">
                    {formatBytes(metrics?.memory?.swap_used_bytes ?? 0)} / {formatBytes(metrics?.memory?.swap_total_bytes ?? 0)}
                  </div>
                </div>
              </div>
              {metrics?.top_memory?.length ? (
                <div className="mt-4 text-xs">
                  <div className="mb-2 flex items-center justify-between text-[var(--app-text-muted)]">
                    <span>Top Memory Processes</span>
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        className="rounded border border-[var(--app-border)] px-2 py-0.5"
                        onClick={() => onPageChange(Math.max(0, metricsPage - 1))}
                      >
                        Prev
                      </button>
                      <button
                        type="button"
                        className="rounded border border-[var(--app-border)] px-2 py-0.5"
                        onClick={() => onPageChange(metricsPage + 1)}
                      >
                        Next
                      </button>
                    </div>
                  </div>
                  <div className="space-y-2">
                    {metrics.top_memory.map((proc) => (
                      <div key={`mem-${proc.pid}`} className="flex items-center justify-between rounded-lg border border-[var(--app-border)] bg-[var(--app-surface)] px-2 py-2">
                        <span className="truncate">
                          {proc.name} <span className="text-[var(--app-text-muted)]">PID {proc.pid}</span>
                        </span>
                        <span className="text-[var(--app-text)]">{formatBytes(proc.memory_bytes)}</span>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}
            </div>
          )}

          {open === "gpu" && (
            <div className="rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface-muted)]/70 p-4">
              <div className="mb-4 flex items-center justify-between">
                <div>
                  <div className="text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">GPU</div>
                  <div className="text-lg font-semibold text-[var(--app-text)]">
                    {metrics?.gpu?.available ? "Available" : "N/A"}
                  </div>
                </div>
                <div className="rounded-full border border-[var(--app-border)] px-3 py-1 text-xs text-[var(--app-text-muted)]">
                  {metrics?.gpu?.available ? "Active" : "Unavailable"}
                </div>
              </div>
              <div className="rounded-lg border border-[var(--app-border)] bg-[var(--app-surface)] px-3 py-3 text-xs text-[var(--app-text-muted)]">
                {metrics?.gpu?.note ?? "Not available"}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
