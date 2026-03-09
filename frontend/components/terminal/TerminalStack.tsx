"use client";

import { cn } from "@/lib/utils";

import type { LayoutNode } from "./types";

interface TerminalStackProps {
  layout: LayoutNode | null;
  activePaneId: string | null;
  isFullscreen: boolean;
  onActivatePane: (id: string) => void;
  registerPaneRef: (id: string, node: HTMLDivElement | null) => void;
  containerParentRef: React.RefObject<HTMLDivElement>;
}

export function TerminalStack({
  layout,
  activePaneId,
  isFullscreen,
  onActivatePane,
  registerPaneRef,
  containerParentRef,
}: TerminalStackProps) {
  const renderNode = (node: LayoutNode) => {
    return (
        <div
          key={node.id}
          className={cn(
            "relative min-h-0 min-w-0 flex-1 overflow-hidden",
            node.id === activePaneId && "ring-1 ring-[var(--app-accent)]",
          )}
          onClick={() => {
            if (node.id !== activePaneId) {
              onActivatePane(node.id);
            }
          }}
          ref={(el) => registerPaneRef(node.id, el)}
        />
      );
    };

  return (
    <div className="flex min-w-0 w-full flex-1 flex-col">
      <div
        className={cn(
          "relative min-h-0 w-full flex-1 overflow-hidden bg-[var(--term-bg)]",
          !isFullscreen &&
            "m-3 max-md:mx-0 max-md:my-2 rounded-2xl border border-[var(--app-border)] shadow-[0_18px_50px_rgba(0,0,0,0.35)] max-md:rounded-none max-md:border-x-0",
        )}
      >
        {layout ? (
          <div className="flex h-full w-full">{renderNode(layout)}</div>
        ) : null}
        {!layout && (
          <div className="absolute inset-0 flex items-center justify-center">
            <p className="text-sm text-[var(--app-text-muted)]">No active terminal tab</p>
          </div>
        )}
		<div
			ref={containerParentRef}
			className="absolute inset-0 pointer-events-none opacity-0"
			aria-hidden="true"
		/>
      </div>
      <div className="sr-only" />
    </div>
  );
}
