"use client";

import { cn } from "@/lib/utils";

interface MobileStatusStripProps {
	showConnected: boolean;
	isFullscreen: boolean;
}

export function MobileStatusStrip({ showConnected, isFullscreen }: MobileStatusStripProps) {
  if (isFullscreen) return null;

	return (
		<div className="flex items-center justify-end gap-2 border-b border-[var(--app-border)] bg-[var(--app-surface)]/70 px-3 py-2 text-[11px] text-[var(--app-text-muted)] sm:hidden">
			<div className="flex items-center gap-2">
				<span className={cn("h-2 w-2 rounded-full", showConnected ? "bg-emerald-500" : "bg-rose-500")} />
				<span>{showConnected ? "On" : "Off"}</span>
			</div>
		</div>
	);
}
