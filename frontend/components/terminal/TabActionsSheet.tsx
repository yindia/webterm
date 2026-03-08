"use client";

import { Terminal as TerminalIcon } from "lucide-react";

import { Button } from "@/components/ui/button";

import type { SessionInfo } from "./types";

interface TabActionsSheetProps {
  session: SessionInfo | null;
  onClose: () => void;
  onSwitch: (id: string) => void;
  onRename: (id: string) => void;
  onDetach: (id: string) => void;
  onCloseSession: (id: string) => void;
}

export function TabActionsSheet({
  session,
  onClose,
  onSwitch,
  onRename,
  onDetach,
  onCloseSession,
}: TabActionsSheetProps) {
  if (!session) return null;

  return (
    <div className="fixed inset-0 z-40 flex items-end justify-center bg-black/40 p-4 sm:items-center" onClick={onClose}>
      <div
        className="w-full max-w-sm rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface)] p-4 shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-[var(--app-text)]">
          <TerminalIcon className="h-4 w-4" />
          <span className="truncate">{session.name}</span>
        </div>
        <div className="grid gap-2 text-sm">
          <Button
            variant="secondary"
            className="w-full justify-start"
            onClick={() => onSwitch(session.id)}
          >
            Switch to tab
          </Button>
          <Button
            variant="secondary"
            className="w-full justify-start"
            onClick={() => onRename(session.id)}
          >
            Rename tab
          </Button>
          <Button
            variant="secondary"
            className="w-full justify-start"
            onClick={() => onDetach(session.id)}
          >
            Detach tab
          </Button>
          <Button
            variant="destructive"
            className="w-full justify-start"
            onClick={() => onCloseSession(session.id)}
          >
            Close tab
          </Button>
        </div>
        <Button variant="ghost" className="mt-3 w-full" onClick={onClose}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
