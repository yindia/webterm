"use client";

import { ExternalLink, Maximize2, Minimize2, Plus, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

import type { SessionInfo } from "./types";
import { compactSessionName } from "./utils";

interface TabsRowProps {
  sessions: SessionInfo[];
  activeId: string | null;
  dragId: string | null;
  canCreateSession: boolean;
  isFullscreen: boolean;
  renamingId: string | null;
  renamingValue: string;
  onCreateSession: () => void;
  onToggleFullscreen: () => void;
  onSwitchSession: (id: string) => void;
  onDetachSession: (id: string) => void;
  onCloseSession: (id: string) => void;
  onSetDragId: (id: string | null) => void;
  onMoveSession: (sourceId: string, targetId: string) => void;
  onOpenTabActions: (id: string) => void;
  onTabHoldStart: (id: string) => void;
  onTabHoldEnd: (id: string) => void;
  onRenameStart: (id: string) => void;
  onRenameChange: (value: string) => void;
  onRenameSubmit: () => void;
  onRenameCancel: () => void;
}

export function TabsRow({
  sessions,
  activeId,
  dragId,
  canCreateSession,
  isFullscreen,
  renamingId,
  renamingValue,
  onCreateSession,
  onToggleFullscreen,
  onSwitchSession,
  onDetachSession,
  onCloseSession,
  onSetDragId,
  onMoveSession,
  onOpenTabActions,
  onTabHoldStart,
  onTabHoldEnd,
  onRenameStart,
  onRenameChange,
  onRenameSubmit,
  onRenameCancel,
}: TabsRowProps) {
  return (
    <div
      className={cn(
        "relative z-40 flex h-12 w-full shrink-0 items-center gap-2 border-b border-[var(--app-border)] bg-[var(--app-surface)]/70 px-3 backdrop-blur",
        !isFullscreen && "border-t-0",
        "max-sm:h-11 max-sm:px-2",
      )}
    >
      <div className="pointer-events-none absolute left-0 top-0 h-full w-5 bg-gradient-to-r from-[var(--app-surface)]/90 to-transparent sm:hidden" />
      <div className="pointer-events-none absolute right-0 top-0 h-full w-5 bg-gradient-to-l from-[var(--app-surface)]/90 to-transparent sm:hidden" />
      <div className="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto max-sm:scroll-px-2 max-sm:snap-x max-sm:snap-mandatory">
        {sessions.map((s) => (
          
          <div
            key={`tab-${s.id}`}
            role="button"
            tabIndex={0}
            className={cn(
              "flex shrink-0 items-center gap-2 rounded-lg border px-2.5 py-1 text-xs transition",
              s.id === activeId
                ? "border-[var(--app-accent)] bg-[var(--app-accent)]/10 text-[var(--app-text)]"
                : "border-[var(--app-border)] bg-[var(--app-surface)] text-[var(--app-text-muted)] hover:text-[var(--app-text)]",
              "max-sm:snap-start max-sm:gap-1 max-sm:px-2 max-sm:py-0.5 max-sm:text-[10px]",
            )}
            onClick={() => onSwitchSession(s.id)}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                onSwitchSession(s.id);
              }
            }}
            onDoubleClick={() => onRenameStart(s.id)}
            onContextMenu={(event) => {
              event.preventDefault();
              onOpenTabActions(s.id);
            }}
            onPointerDown={() => onTabHoldStart(s.id)}
            onPointerUp={() => onTabHoldEnd(s.id)}
            onPointerLeave={() => onTabHoldEnd(s.id)}
            onPointerCancel={() => onTabHoldEnd(s.id)}
            draggable={renamingId !== s.id}
            onDragStart={() => onSetDragId(s.id)}
            onDragOver={(event) => event.preventDefault()}
            onDrop={() => {
              if (dragId) onMoveSession(dragId, s.id);
              onSetDragId(null);
            }}
            onDragEnd={() => onSetDragId(null)}
          >
            {renamingId === s.id ? (
              <input
                className="h-5 w-[120px] rounded-md border border-[var(--app-border)] bg-[var(--app-surface)] px-2 text-[10px] text-[var(--app-text)] outline-none sm:w-[160px] sm:text-xs"
                value={renamingValue}
                autoFocus
                onClick={(event) => event.stopPropagation()}
                onChange={(event) => onRenameChange(event.target.value)}
                onBlur={onRenameSubmit}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    onRenameSubmit();
                  }
                  if (event.key === "Escape") {
                    event.preventDefault();
                    onRenameCancel();
                  }
                }}
              />
            ) : (
              <>
                <span className="max-w-[140px] truncate max-sm:hidden">{s.name}</span>
                <span className="max-w-[90px] truncate sm:hidden">{compactSessionName(s.name)}</span>
              </>
            )}
            <span
              className={cn(
                "h-1.5 w-1.5 rounded-full",
                s.id === activeId ? "bg-[var(--app-accent)]" : "bg-[var(--app-text-muted)]",
              )}
            />
            <button
              type="button"
              className="text-[var(--app-text-muted)] hover:text-[var(--app-text)] max-sm:hidden"
              onClick={(event) => {
                event.stopPropagation();
                onDetachSession(s.id);
              }}
              aria-label={`Detach ${s.name}`}
            >
              <ExternalLink className="h-3 w-3" />
            </button>
            <button
              type="button"
              className="text-[var(--app-text-muted)] hover:text-[var(--app-text)]"
              onClick={(event) => {
                event.stopPropagation();
                onCloseSession(s.id);
              }}
              aria-label={`Close ${s.name}`}
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        ))}
      </div>
      <Button
        variant="secondary"
        size="sm"
        className={cn(
          "h-7 w-7 shrink-0 p-0",
          !canCreateSession && "cursor-not-allowed opacity-50",
        )}
        onClick={onCreateSession}
        aria-label="New terminal"
        aria-disabled={!canCreateSession}
      >
        <Plus className="h-4 w-4" />
      </Button>
      <div className="flex items-center gap-2 text-xs text-[var(--app-text-muted)]">
        <span className="hidden sm:inline">Alt+Shift+F</span>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 w-7 p-0 text-[var(--app-text-muted)] hover:text-[var(--app-text)]"
          onClick={onToggleFullscreen}
          aria-label="Toggle fullscreen"
        >
          {isFullscreen ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  );
}
