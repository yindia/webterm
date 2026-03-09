"use client";

import { Activity, Terminal as TerminalIcon } from "lucide-react";

import { cn } from "@/lib/utils";

interface HeaderBarProps {
  showConnected: boolean;
  isFullscreen: boolean;
  onMonitoringOpen: () => void;
}

export function HeaderBar({
  showConnected,
  isFullscreen,
  onMonitoringOpen,
}: HeaderBarProps) {
  return (
    <header
      className={cn(
        "flex h-14 w-full shrink-0 items-center justify-between gap-3 border-b border-[var(--app-border)] bg-[var(--app-surface)]/80 px-4 backdrop-blur",
        isFullscreen && "hidden",
        "max-sm:h-auto max-sm:flex-wrap max-sm:gap-2 max-sm:px-3 max-sm:py-2",
      )}
    >
      <div className="flex items-center gap-2 text-[var(--app-accent)]">
        <TerminalIcon className="h-5 w-5" />
        <span className="text-sm font-semibold tracking-tight">webterm</span>
        <span className="hidden rounded-full border border-[var(--app-border)] px-2 py-0.5 text-[10px] uppercase tracking-[0.2em] text-[var(--app-text-muted)] sm:inline">
          Live
        </span>
      </div>
      <div className="flex items-center gap-3 max-sm:flex-wrap max-sm:justify-end max-sm:gap-2">
        <button
          type="button"
          className="flex items-center gap-1 rounded-full border border-[var(--app-border)] px-2 py-1 text-[11px] text-[var(--app-text-muted)] max-sm:px-1.5 max-sm:py-0.5"
          onClick={onMonitoringOpen}
        >
          <Activity className="h-3.5 w-3.5" />
          <span className="max-sm:hidden">Monitor</span>
        </button>
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-full border border-[var(--app-border)] px-2.5 py-0.5 text-xs font-medium max-sm:px-2 max-sm:py-0.5",
            showConnected
              ? "bg-emerald-500/10 text-emerald-600"
              : "bg-rose-500/10 text-rose-500",
          )}
        >
          <span
            className={cn(
              "h-1.5 w-1.5 rounded-full",
              showConnected ? "bg-emerald-500" : "bg-rose-500",
            )}
          />
          <span className="max-sm:hidden">{showConnected ? "Connected" : "Disconnected"}</span>
        </span>
      </div>
    </header>
  );
}
