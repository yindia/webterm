"use client";

import { Cpu, MemoryStick } from "lucide-react";

import { cn } from "@/lib/utils";

import type { MetricsSnapshot } from "./types";
import { formatBytes } from "./utils";

interface MobileStatusStripProps {
  metrics: MetricsSnapshot | null;
  showConnected: boolean;
  isFullscreen: boolean;
}

export function MobileStatusStrip({ metrics, showConnected, isFullscreen }: MobileStatusStripProps) {
  if (isFullscreen) return null;

  return (
    <div className="flex items-center justify-between gap-2 border-b border-[var(--app-border)] bg-[var(--app-surface)]/70 px-3 py-2 text-[11px] text-[var(--app-text-muted)] sm:hidden">
      <div className="flex items-center gap-2">
        <Cpu className="h-3.5 w-3.5" />
        <span>{metrics?.cpu?.available ? `${metrics.cpu.usage_percent.toFixed(0)}%` : "N/A"}</span>
      </div>
      <div className="flex items-center gap-2">
        <MemoryStick className="h-3.5 w-3.5" />
        <span className="max-w-[90px] truncate whitespace-nowrap">
          {metrics?.memory?.available ? formatBytes(metrics.memory.used_bytes) : "N/A"}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <span className={cn("h-2 w-2 rounded-full", showConnected ? "bg-emerald-500" : "bg-rose-500")} />
        <span>{showConnected ? "On" : "Off"}</span>
      </div>
    </div>
  );
}
