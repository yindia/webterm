"use client";

import { Settings, X } from "lucide-react";

interface SettingsModalProps {
  open: boolean;
  fontSize: number;
  cursorStyle: string;
  cursorStyles: readonly string[];
  onClose: () => void;
  onFontSizeChange: (value: number) => void;
  onCursorStyleChange: (value: string) => void;
}

export function SettingsModal({
  open,
  fontSize,
  cursorStyle,
  cursorStyles,
  onClose,
  onFontSizeChange,
  onCursorStyleChange,
}: SettingsModalProps) {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm">
      <div className="w-full max-w-sm rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface)] p-5 shadow-2xl">
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-2 text-[var(--app-text)]">
            <Settings className="h-4 w-4" />
            <span className="text-sm font-semibold">Terminal Settings</span>
          </div>
          <button
            type="button"
            className="rounded-md p-1 text-[var(--app-text-muted)] hover:text-[var(--app-text)]"
            onClick={onClose}
            aria-label="Close settings"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-4 text-sm">
          <div>
            <label className="mb-1 block text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">
              Font Size
            </label>
            <div className="flex items-center gap-3">
              <input
                type="range"
                min={10}
                max={22}
                value={fontSize}
                onChange={(event) => onFontSizeChange(Number(event.target.value))}
                className="w-full"
              />
              <span className="w-8 text-right text-[var(--app-text)]">{fontSize}</span>
            </div>
          </div>

          <div>
            <label className="mb-1 block text-xs uppercase tracking-[0.2em] text-[var(--app-text-muted)]">
              Cursor Style
            </label>
            <select
              value={cursorStyle}
              onChange={(event) => onCursorStyleChange(event.target.value)}
              className="w-full rounded-lg border border-[var(--app-border)] bg-[var(--app-surface-muted)] px-3 py-2 text-[var(--app-text)]"
            >
              {cursorStyles.map((style) => (
                <option key={style} value={style}>
                  {style}
                </option>
              ))}
            </select>
          </div>

        </div>
      </div>
    </div>
  );
}
