"use client";

import { Settings } from "lucide-react";

import { Button } from "@/components/ui/button";

import type { SessionInfo } from "./types";

interface StatusBarProps {
  showConnected: boolean;
  activeSession: SessionInfo | null;
  onOpenSettings: () => void;
}

export function StatusBar({ showConnected, activeSession, onOpenSettings }: StatusBarProps) {
  return (
    <div className="fixed bottom-0 left-0 right-0 z-30 flex h-8 items-center justify-between border-t border-[var(--app-border)] bg-[var(--app-surface)]/90 px-4 text-xs text-[var(--app-text-muted)] backdrop-blur">
      <span>
        {showConnected ? "● Connected" : "○ Disconnected"}
        {activeSession ? ` — ${activeSession.name}` : ""}
      </span>
      <div className="flex items-center gap-3">
        <span className="hidden sm:inline">
          Alt+Shift+N new tab · Alt+Shift+←/→ switch · Alt+Shift+F fullscreen
        </span>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 w-7 p-0 text-[var(--app-text-muted)] hover:text-[var(--app-text)]"
          onClick={onOpenSettings}
          aria-label="Open settings"
        >
          <Settings className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
