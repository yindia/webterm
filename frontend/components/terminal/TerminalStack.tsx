"use client";

import { cn } from "@/lib/utils";

import type { SessionInfo } from "./types";

interface TerminalStackProps {
  sessions: SessionInfo[];
  isFullscreen: boolean;
  containerParentRef: React.RefObject<HTMLDivElement>;
}

export function TerminalStack({ sessions, isFullscreen, containerParentRef }: TerminalStackProps) {
  return (
    <div className="flex min-w-0 w-full flex-1 flex-col">
      <div
        className={cn(
          "relative min-h-0 w-full flex-1 overflow-hidden bg-[var(--term-bg)]",
          !isFullscreen &&
            "m-3 max-md:mx-0 max-md:my-2 rounded-2xl border border-[var(--app-border)] shadow-[0_18px_50px_rgba(0,0,0,0.35)] max-md:rounded-none max-md:border-x-0",
        )}
      >
        <div
          ref={containerParentRef}
          className={cn("terminal-layer bg-[var(--term-bg)]", isFullscreen && "fullscreen")}
        />
        {sessions.length === 0 && (
          <div className="absolute inset-0 flex items-center justify-center">
            <p className="text-sm text-[var(--app-text-muted)]">No active terminal tab</p>
          </div>
        )}
      </div>
      <div className="sr-only" />
    </div>
  );
}
