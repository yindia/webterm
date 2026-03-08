"use client";

import { Terminal as TerminalIcon } from "lucide-react";

import { cn } from "@/lib/utils";

import type { SessionInfo } from "./types";

interface QuickSwitcherProps {
  open: boolean;
  query: string;
  sessions: SessionInfo[];
  activeId: string | null;
  onClose: () => void;
  onQueryChange: (value: string) => void;
  onSelect: (id: string) => void;
}

export function QuickSwitcher({
  open,
  query,
  sessions,
  activeId,
  onClose,
  onQueryChange,
  onSelect,
}: QuickSwitcherProps) {
  if (!open) return null;

  const normalized = query.trim().toLowerCase();
  const matches = normalized
    ? sessions.filter((s) => s.name.toLowerCase().includes(normalized))
    : sessions;

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-black/40 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className="mt-16 w-full max-w-md rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface)] p-4 shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center gap-2 text-sm font-semibold text-[var(--app-text)]">
          <TerminalIcon className="h-4 w-4" />
          <span>Quick Switch</span>
        </div>
        <input
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          autoFocus
          placeholder="Type a session name"
          className="mt-3 w-full rounded-lg border border-[var(--app-border)] bg-[var(--app-surface-muted)] px-3 py-2 text-sm text-[var(--app-text)] outline-none focus:border-[var(--app-accent)] focus:ring-1 focus:ring-[var(--app-accent)]"
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              onClose();
            }
            if (event.key === "Enter" && matches[0]) {
              event.preventDefault();
              onSelect(matches[0].id);
            }
          }}
        />
        <div className="mt-3 max-h-64 overflow-y-auto">
          {matches.length === 0 ? (
            <div className="rounded-lg border border-[var(--app-border)] bg-[var(--app-surface-muted)] px-3 py-2 text-xs text-[var(--app-text-muted)]">
              No matching sessions
            </div>
          ) : (
            matches.map((s) => (
              <button
                key={s.id}
                type="button"
                className={cn(
                  "mb-2 flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left text-sm",
                  s.id === activeId
                    ? "border-[var(--app-accent)] bg-[var(--app-accent)]/10 text-[var(--app-text)]"
                    : "border-[var(--app-border)] bg-[var(--app-surface)] text-[var(--app-text-muted)] hover:text-[var(--app-text)]",
                )}
                onClick={() => onSelect(s.id)}
              >
                <span className="truncate">{s.name}</span>
                {s.id === activeId && <span className="text-[10px] uppercase tracking-[0.2em]">Active</span>}
              </button>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
