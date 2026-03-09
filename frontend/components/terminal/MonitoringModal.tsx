"use client";

import { Activity, AlertTriangle, CheckCircle2, Clock, X } from "lucide-react";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import type { MonitoringEvent, MonitoringSessionSummary } from "./types";

interface MonitoringModalProps {
  open: boolean;
  sessions: MonitoringSessionSummary[];
  selectedId: string | null;
  events: MonitoringEvent[];
  notifyTitle: string;
  notifyMessage: string;
  onClose: () => void;
  onSelect: (id: string) => void;
  onNotifyTitleChange: (value: string) => void;
  onNotifyMessageChange: (value: string) => void;
  onSendNotify: () => void;
  notifying: boolean;
}

function Sparkline({ values }: { values: number[] }) {
  if (values.length < 2) {
    return <span className="text-[var(--app-text-muted)]">-</span>;
  }
  const width = 120;
  const height = 28;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const points = values
    .map((value, idx) => {
      const x = (idx / (values.length - 1)) * width;
      const y = height - ((value - min) / range) * height;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");

  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} aria-hidden="true">
      <polyline
        points={points}
        fill="none"
        stroke="var(--app-accent)"
        strokeWidth="1.5"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}

function attentionClass(attention: string) {
  switch (attention) {
    case "error":
      return "bg-rose-500/10 text-rose-500";
    case "wait":
      return "bg-amber-500/10 text-amber-500";
    case "high":
      return "bg-sky-500/10 text-sky-400";
    case "done":
      return "bg-emerald-500/10 text-emerald-500";
    case "idle":
      return "bg-slate-500/10 text-slate-400";
    default:
      return "bg-slate-500/10 text-slate-400";
  }
}

function attentionIcon(attention: string) {
  if (attention === "error") return <AlertTriangle className="h-3.5 w-3.5" />;
  if (attention === "done") return <CheckCircle2 className="h-3.5 w-3.5" />;
  if (attention === "wait") return <Clock className="h-3.5 w-3.5" />;
  return <Activity className="h-3.5 w-3.5" />;
}

export function MonitoringModal({
  open,
  sessions,
  selectedId,
  events,
  notifyTitle,
  notifyMessage,
  onClose,
  onSelect,
  onNotifyTitleChange,
  onNotifyMessageChange,
  onSendNotify,
  notifying,
}: MonitoringModalProps) {
  if (!open) return null;

  const selected = sessions.find((session) => session.id === selectedId) ?? null;


  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className="w-full max-w-6xl rounded-3xl border border-[var(--app-border)] bg-[var(--app-surface)] p-6 shadow-[0_30px_80px_rgba(0,0,0,0.35)]"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-2 text-[var(--app-text)]">
            <Activity className="h-4 w-4" />
            <span className="text-sm font-semibold">Session Monitor</span>
          </div>
          <button
            type="button"
            className="rounded-md p-1 text-[var(--app-text-muted)] hover:text-[var(--app-text)]"
            onClick={onClose}
            aria-label="Close monitoring"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="grid gap-6 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]">
          <div className="space-y-3">
            <div className="rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface-muted)]/70 px-3 py-2 text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">
              Active Sessions
            </div>
            <div className="space-y-2">
              {sessions.length === 0 && (
                <div className="rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface)] p-6 text-sm text-[var(--app-text-muted)]">
                  No monitoring data yet.
                </div>
              )}
              {sessions.map((session) => (
                <button
                  key={session.id}
                  type="button"
                  onClick={() => onSelect(session.id)}
                  className={cn(
                    "flex w-full items-center justify-between gap-3 rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface)] px-3 py-3 text-left transition",
                    selectedId === session.id && "border-[var(--app-accent)]/60 bg-[var(--app-surface-muted)]/60",
                  )}
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm font-semibold text-[var(--app-text)]">
                      <span className="truncate">{session.name || session.id}</span>
                      <span className={cn("inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] uppercase tracking-[0.2em]", attentionClass(session.attention))}>
                        {attentionIcon(session.attention)}
                        {session.attention || "low"}
                      </span>
                    </div>
                    <div className="mt-1 text-xs text-[var(--app-text-muted)]">
                      {session.command || "shell"}
                    </div>
                  </div>
                  <div className="shrink-0">
                    <Sparkline values={(session.activity ?? []).map((point) => point.score)} />
                  </div>
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-4">
            <div className="rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface-muted)]/70 px-3 py-2 text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">
              {selected ? `Session: ${selected.name || selected.id}` : "Details"}
            </div>

            <div className="rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface)] p-4">
              <div className="text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">Notify Session</div>
              <div className="mt-3 space-y-2">
                <input
                  className="w-full rounded-xl border border-[var(--app-border)] bg-[var(--app-surface-muted)]/60 px-3 py-2 text-xs text-[var(--app-text)] outline-none focus:border-[var(--app-accent)]"
                  placeholder="Title (required)"
                  value={notifyTitle}
                  onChange={(event) => onNotifyTitleChange(event.target.value)}
                />
                <textarea
                  className="h-20 w-full rounded-xl border border-[var(--app-border)] bg-[var(--app-surface-muted)]/60 px-3 py-2 text-xs text-[var(--app-text)] outline-none focus:border-[var(--app-accent)]"
                  placeholder="Message"
                  value={notifyMessage}
                  onChange={(event) => onNotifyMessageChange(event.target.value)}
                />
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={!selected || notifying || notifyTitle.trim() === ""}
                  onClick={onSendNotify}
                >
                  {notifying ? "Sending..." : "Send Notification"}
                </Button>
              </div>
            </div>

            <div className="rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface)] p-4">
              <div className="text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">Events</div>
              <div className="mt-3 space-y-2 text-xs">
                {events.length === 0 && <div className="text-[var(--app-text-muted)]">No events yet.</div>}
                {events.map((event, idx) => (
                  <div key={`${event.timestamp}-${idx}`} className="rounded-lg border border-[var(--app-border)] bg-[var(--app-surface-muted)]/60 px-2 py-2">
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-[var(--app-text)]">{event.title}</span>
                      <span className="text-[var(--app-text-muted)]">{new Date(event.timestamp).toLocaleTimeString()}</span>
                    </div>
                    {event.message && <div className="mt-1 text-[var(--app-text-muted)]">{event.message}</div>}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
