"use client";

import { cn } from "@/lib/utils";

export interface ToastItem {
  id: string;
  message: string;
  tone?: "success" | "error" | "info";
}

interface ToastStackProps {
  toasts: ToastItem[];
}

export function ToastStack({ toasts }: ToastStackProps) {
  if (toasts.length === 0) return null;

  return (
    <div className="fixed right-4 top-20 z-50 flex w-[280px] flex-col gap-2 sm:w-[320px]">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={cn(
            "rounded-xl border border-[var(--app-border)] bg-[var(--app-surface)] px-3 py-2 text-xs text-[var(--app-text)] shadow-[0_15px_40px_rgba(0,0,0,0.2)]",
            toast.tone === "success" && "border-emerald-500/40 text-emerald-600",
            toast.tone === "error" && "border-rose-500/40 text-rose-500",
          )}
        >
          {toast.message}
        </div>
      ))}
    </div>
  );
}
