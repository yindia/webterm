"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";

import { HeaderBar } from "@/components/terminal/HeaderBar";
import { LoginScreen } from "@/components/terminal/LoginScreen";
import { MonitoringModal } from "@/components/terminal/MonitoringModal";
import { MobileStatusStrip } from "@/components/terminal/MobileStatusStrip";
import { QuickSwitcher } from "@/components/terminal/QuickSwitcher";
import { SettingsModal } from "@/components/terminal/SettingsModal";
import { StatusBar } from "@/components/terminal/StatusBar";
import { TabsRow } from "@/components/terminal/TabsRow";
import { TabActionsSheet } from "@/components/terminal/TabActionsSheet";
import { TerminalStack } from "@/components/terminal/TerminalStack";
import { ToastStack, ToastItem } from "@/components/terminal/ToastStack";
import {
	LayoutNode,
	LayoutPaneNode,
	MonitoringEvent,
	MonitoringSessionSummary,
	SessionInfo,
} from "@/components/terminal/types";

import type { Terminal } from "xterm";
import type { FitAddon } from "xterm-addon-fit";
import type { WebLinksAddon } from "xterm-addon-web-links";

interface SessionTerminal {
	terminal: Terminal;
	fitAddon: FitAddon;
	container: HTMLDivElement;
	socket: WebSocket;
	replaying: boolean;
	queue: Uint8Array[];
	canResize: boolean;
}

const TERM_OPTIONS = {
  cursorBlink: true,
  scrollback: 5000,
  fontSize: 13,
  fontFamily:
    "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, Courier New, monospace",
} as const;

const TERM_THEME = {
  background: "#030712",
  foreground: "#dbe5f5",
  cursor: "#7dd3fc",
} as const;

const CURSOR_STYLES = ["block", "bar", "underline"] as const;
const MAX_SESSIONS = 5;

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function getCookie(name: string): string {
  const match = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
  return match ? decodeURIComponent(match[1]) : "";
}

function decodeBase64ToBytes(data: string): Uint8Array {
	if (!data) return new Uint8Array();
	const chunkSize = 16384;
	const chunks: Uint8Array[] = [];
	let total = 0;
	for (let i = 0; i < data.length; i += chunkSize) {
		const slice = data.slice(i, i + chunkSize);
		const raw = atob(slice);
		const bytes = new Uint8Array(raw.length);
		for (let j = 0; j < raw.length; j += 1) {
			bytes[j] = raw.charCodeAt(j);
		}
		chunks.push(bytes);
		total += bytes.length;
	}
	const out = new Uint8Array(total);
	let offset = 0;
	for (const chunk of chunks) {
		out.set(chunk, offset);
		offset += chunk.length;
	}
	return out;
}

function wsUrl(sessionId: string, csrf: string, cols: number, rows: number): string {
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	const params = new URLSearchParams({
		csrf,
		cols: String(cols),
		rows: String(rows),
	});
	return `${protocol}//${window.location.host}/api/terminal/ws/${sessionId}?${params.toString()}`;
}

interface PaneInfo {
  id: string;
  sessionId: string;
}

type TerminalLayoutNode = LayoutNode;
type TerminalPaneNode = LayoutPaneNode;

function createPaneNode(sessionId: string, id?: string): TerminalPaneNode {
  return {
    type: "pane",
    id: id ?? `pane-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    sessionId,
  };
}

function flattenLayout(layout: TerminalLayoutNode | null | undefined): PaneInfo[] {
  if (!layout) return [];
  return [{ id: layout.id, sessionId: layout.sessionId }];
}

function findPaneById(layout: TerminalLayoutNode, paneId: string): TerminalPaneNode | null {
  return layout.id === paneId ? layout : null;
}

function getFirstPaneId(layout: TerminalLayoutNode): string {
  return layout.id;
}


/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

export default function Page() {
  /* ---- auth state ---- */
  const [authenticated, setAuthenticated] = useState(false);
  const [password, setPassword] = useState("");
  const [loginError, setLoginError] = useState("");
  const [loggingIn, setLoggingIn] = useState(false);

  /* ---- session state ---- */
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [serverAlive, setServerAlive] = useState(true);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [fontSize, setFontSize] = useState(13);
  const [cursorStyle, setCursorStyle] = useState<(typeof CURSOR_STYLES)[number]>("block");
  const [dragId, setDragId] = useState<string | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [monitoringOpen, setMonitoringOpen] = useState(false);
  const [monitoringSessions, setMonitoringSessions] = useState<MonitoringSessionSummary[]>([]);
  const [monitoringActiveId, setMonitoringActiveId] = useState<string | null>(null);
  const [monitoringEvents, setMonitoringEvents] = useState<Record<string, MonitoringEvent[]>>({});
  const [monitoringNotifyTitle, setMonitoringNotifyTitle] = useState("");
  const [monitoringNotifyMessage, setMonitoringNotifyMessage] = useState("");
  const [monitoringNotifying, setMonitoringNotifying] = useState(false);
  const monitoringStreamRef = useRef<EventSource | null>(null);
  const [detachedSessionId, setDetachedSessionId] = useState<string | null>(null);
  const [isDetached, setIsDetached] = useState(false);
  const [tabActionsId, setTabActionsId] = useState<string | null>(null);
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renamingValue, setRenamingValue] = useState("");
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [quickSwitchOpen, setQuickSwitchOpen] = useState(false);
  const [quickQuery, setQuickQuery] = useState("");
  const [idleTimeoutSec, setIdleTimeoutSec] = useState(0);
  const [layoutBySession, setLayoutBySession] = useState<Record<string, TerminalLayoutNode>>({});
  const [activePaneBySession, setActivePaneBySession] = useState<Record<string, string>>({});

  /* ---- refs ---- */
  const csrfRef = useRef("");
  const terminalsRef = useRef<Map<string, SessionTerminal>>(new Map());
  const containerParentRef = useRef<HTMLDivElement | null>(null);
  const activeIdRef = useRef<string | null>(null);
	const timersRef = useRef<Set<ReturnType<typeof setTimeout>>>(new Set());
	const fitTimersRef = useRef<Map<string, number>>(new Map());
	const fitAttemptsRef = useRef<Map<string, number>>(new Map());
	const tabHoldTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
	const mountedRef = useRef(true);
	const lastActivityRef = useRef<Map<string, number>>(new Map());
	const idleWarnedRef = useRef<Set<string>>(new Set());
	const paneRefs = useRef<Map<string, HTMLDivElement>>(new Map());
	const activeResizeObserverRef = useRef<ResizeObserver | null>(null);
	const activePaneIdRef = useRef<string | null>(null);
	const activeSessionIdRef = useRef<string | null>(null);

  const activeLayout = useMemo(
    () => (activeId ? layoutBySession[activeId] ?? null : null),
    [activeId, layoutBySession],
  );
  const activePanes = useMemo(() => flattenLayout(activeLayout), [activeLayout]);
  const activePaneId = useMemo(() => {
    if (!activeId || !activeLayout) return null;
    const preferred = activePaneBySession[activeId];
    if (preferred && findPaneById(activeLayout, preferred)) return preferred;
    return getFirstPaneId(activeLayout);
  }, [activeId, activeLayout, activePaneBySession]);
  const activePaneSessionId = useMemo(() => {
    if (!activeLayout || !activePaneId) return null;
    const pane = findPaneById(activeLayout, activePaneId);
    return pane ? pane.sessionId : null;
  }, [activeLayout, activePaneId]);

  /* keep activeIdRef in sync */
  useEffect(() => {
    activeIdRef.current = activeId;
  }, [activeId]);

  useEffect(() => {
    activePaneIdRef.current = activePaneId;
  }, [activePaneId]);

  useEffect(() => {
    activeSessionIdRef.current = activePaneSessionId;
  }, [activePaneSessionId]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const sessionId = params.get("session");
    const detached = params.get("detached") === "1" && sessionId;
    setDetachedSessionId(sessionId);
    setIsDetached(!!detached);
  }, []);

  useEffect(() => {
    const storedFont = window.localStorage.getItem("webterm-font-size");
    const storedCursor = window.localStorage.getItem("webterm-cursor-style");
    if (storedFont) {
      const parsed = Number(storedFont);
      if (!Number.isNaN(parsed) && parsed >= 10 && parsed <= 22) {
        setFontSize(parsed);
      }
    }
    if (storedCursor && CURSOR_STYLES.includes(storedCursor as (typeof CURSOR_STYLES)[number])) {
      setCursorStyle(storedCursor as (typeof CURSOR_STYLES)[number]);
    }
  }, []);

  /* ---------------------------------------------------------------- */
  /*  Xterm dynamic import cache                                       */
  /* ---------------------------------------------------------------- */

	const xtermModRef = useRef<
		{ Terminal: typeof Terminal; FitAddon: typeof FitAddon; WebLinksAddon: typeof WebLinksAddon } | null
	>(null);

	const loadXterm = useCallback(async () => {
		if (xtermModRef.current) return xtermModRef.current;
		const xtermMod = await import("xterm");
		const fitMod = await import("xterm-addon-fit");
		const webLinksMod = await import("xterm-addon-web-links");
		xtermModRef.current = {
			Terminal: xtermMod.Terminal,
			FitAddon: fitMod.FitAddon,
			WebLinksAddon: webLinksMod.WebLinksAddon,
		};
		return xtermModRef.current;
	}, []);

  /* ---------------------------------------------------------------- */
  /*  Managed timers                                                   */
  /* ---------------------------------------------------------------- */

  const safeTimeout = useCallback((fn: () => void, ms: number) => {
    const id = setTimeout(() => {
      timersRef.current.delete(id);
      if (mountedRef.current) fn();
    }, ms);
    timersRef.current.add(id);
    return id;
  }, []);

  const pushToast = useCallback(
    (message: string, tone: ToastItem["tone"] = "info") => {
      const id = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
      setToasts((prev) => [...prev, { id, message, tone }]);
      safeTimeout(() => {
        setToasts((prev) => prev.filter((toast) => toast.id !== id));
      }, 2500);
    },
    [safeTimeout],
  );

  const getCSRFToken = useCallback(() => {
    return csrfRef.current || getCookie("webterm_csrf");
  }, []);

	const markActivity = useCallback((id: string) => {
		lastActivityRef.current.set(id, Date.now());
		idleWarnedRef.current.delete(id);
	}, []);

	const sendSocketMessage = useCallback((id: string, payload: Record<string, unknown>) => {
		const st = terminalsRef.current.get(id);
		if (!st || st.socket.readyState !== WebSocket.OPEN) return false;
		try {
			st.socket.send(JSON.stringify(payload));
			return true;
		} catch (err) {
			void err;
			return false;
		}
	}, []);

	const sendFocusEvent = useCallback(
		(id: string, focused: boolean) => {
			const seq = focused ? "\u001b[I" : "\u001b[O";
			sendSocketMessage(id, { type: "input", data: seq });
		},
		[sendSocketMessage],
	);

	const postInput = useCallback(
		async (id: string, input: string) => {
			const csrf = getCSRFToken();
			if (!csrf || !input) return;
			if (sendSocketMessage(id, { type: "input", data: input })) return;
			try {
				await fetch(`/api/terminal/input/${id}`, {
          method: "POST",
          headers: {
            "Content-Type": "text/plain",
            "X-CSRF-Token": csrf,
          },
          credentials: "include",
          body: input,
        });
      } catch (err) {
        void err;
      }
		},
		[getCSRFToken, sendSocketMessage],
	);

	const postResize = useCallback(
		async (id: string, cols: number, rows: number) => {
			const csrf = getCSRFToken();
			if (!csrf) return;
			if (sendSocketMessage(id, { type: "resize", cols, rows })) return;
			try {
				await fetch(`/api/terminal/resize/${id}`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": csrf,
          },
          credentials: "include",
          body: JSON.stringify({ cols, rows }),
        });
      } catch (err) {
        void err;
      }
		},
		[getCSRFToken, sendSocketMessage],
	);

	const scheduleFit = useCallback(
		(id: string, force = false) => {
			const st = terminalsRef.current.get(id);
			if (!st || (!st.canResize && !force)) return;
			const existing = fitTimersRef.current.get(id);
			if (existing) {
				window.clearTimeout(existing);
				fitTimersRef.current.delete(id);
			}
			const timeoutId = window.setTimeout(() => {
				fitTimersRef.current.delete(id);
				if (!st.container.isConnected) return;
				const host = st.container.parentElement ?? st.container;
				const { width, height } = host.getBoundingClientRect();
			if (width < 2 || height < 2) {
				const attempts = fitAttemptsRef.current.get(id) ?? 0;
				if (attempts < 8) {
					fitAttemptsRef.current.set(id, attempts + 1);
					scheduleFit(id, force);
				}
				return;
			}
				fitAttemptsRef.current.delete(id);
				try {
					st.fitAddon.fit();
				} catch {
					/* ignore */
				}
				const cols = st.terminal.cols ?? 0;
				const rows = st.terminal.rows ?? 0;
				if (cols > 0 && rows > 0) {
					void postResize(id, cols, rows);
				}
				st.terminal.focus();
		}, 80);
		fitTimersRef.current.set(id, timeoutId);
		},
		[postResize],
	);

	const syncTerminalSize = useCallback(
		(id: string) => {
			const st = terminalsRef.current.get(id);
			if (!st) return;
			const dims = st.fitAddon.proposeDimensions?.();
			if (!dims || dims.cols <= 0 || dims.rows <= 0) {
				scheduleFit(id, true);
				return;
			}
			try {
				st.terminal.resize(dims.cols, dims.rows);
			} catch {
				/* ignore */
			}
			void postResize(id, dims.cols, dims.rows);
			st.terminal.focus();
		},
		[postResize, scheduleFit],
	);

  const moveSession = useCallback((sourceId: string, targetId: string) => {
    if (sourceId === targetId) return;
    setSessions((prev) => {
      const sourceIndex = prev.findIndex((s) => s.id === sourceId);
      const targetIndex = prev.findIndex((s) => s.id === targetId);
      if (sourceIndex === -1 || targetIndex === -1) return prev;
      const next = [...prev];
      const [item] = next.splice(sourceIndex, 1);
      next.splice(targetIndex, 0, item);
      return next;
    });
  }, []);

  const openDetached = useCallback((id: string) => {
    const url = new URL(window.location.href);
    url.searchParams.set("session", id);
    url.searchParams.set("detached", "1");
    window.open(url.toString(), `_blank`, "noopener,noreferrer");
  }, []);

  const renameSession = useCallback(
    async (id: string, name: string) => {
      const csrf = getCSRFToken();
      if (!csrf) return;
      try {
        const res = await fetch(`/api/terminal/sessions/${id}`, {
          method: "PATCH",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": csrf,
          },
          credentials: "include",
          body: JSON.stringify({ name }),
        });
        if (!res.ok) {
          pushToast("Rename failed", "error");
          return;
        }
        const data: { id: string; name: string } = await res.json();
        setSessions((prev) => prev.map((s) => (s.id === data.id ? { ...s, name: data.name } : s)));
        pushToast("Tab renamed", "success");
      } catch {
        pushToast("Rename failed", "error");
      }
    },
    [getCSRFToken, pushToast],
  );

  /* ---------------------------------------------------------------- */
  /*  Terminal lifecycle                                                */
  /* ---------------------------------------------------------------- */

  const switchToSession = useCallback((id: string) => {
    setActiveId(id);
  }, []);

  const dropSessionLocal = useCallback(
    (id: string) => {
		const st = terminalsRef.current.get(id);
		if (st) {
			try {
				st.socket.close();
			} catch (err) {
				void err;
			}
        st.terminal.dispose();
        st.container.parentNode?.removeChild(st.container);
        terminalsRef.current.delete(id);
      }

      setSessions((prev) => {
        const next = prev.filter((s) => s.id !== id);
        if (activeIdRef.current === id) {
          if (next.length > 0) {
            switchToSession(next[0].id);
          } else {
            setActiveId(null);
            setConnected(false);
          }
        }
        return next;
      });
      setLayoutBySession((prev) => {
        if (!(id in prev)) return prev;
        const next = { ...prev };
        delete next[id];
        return next;
      });
      setActivePaneBySession((prev) => {
        if (!(id in prev)) return prev;
        const next = { ...prev };
        delete next[id];
        return next;
      });
    },
    [switchToSession],
  );

  const attachSession = useCallback(
    async (session: SessionInfo) => {
      const mods = await loadXterm();
      const parent = containerParentRef.current;
      if (!parent) return;

		const terminal = new mods.Terminal({
			...TERM_OPTIONS,
			fontSize,
			cursorStyle,
			theme: TERM_THEME,
		});
		const fitAddon = new mods.FitAddon();
		terminal.loadAddon(fitAddon);
		terminal.loadAddon(new mods.WebLinksAddon());

      const container = document.createElement("div");
      container.className = "terminal-shell";
      container.style.position = "absolute";
      container.style.inset = "0";
      container.style.zIndex = "0";
      container.style.visibility = "hidden";
      parent.appendChild(container);

		terminal.open(container);
		terminal.attachCustomKeyEventHandler((event) => {
			if (event.key === "Tab") {
				event.preventDefault();
			}
			return true;
		});
		try {
			fitAddon.fit();
		} catch {
			/* ignore */
		}
		container.addEventListener("mousedown", () => {
			try {
				terminal.focus();
        } catch {
          /* ignore */
        }
      });

		const csrf = getCSRFToken();
		const proposed = fitAddon.proposeDimensions?.();
		const initialCols = Math.max(2, proposed?.cols ?? terminal.cols ?? 80);
		const initialRows = Math.max(1, proposed?.rows ?? terminal.rows ?? 24);
		if (proposed) {
			try {
				terminal.resize(initialCols, initialRows);
			} catch {
				/* ignore */
			}
		}
		const ws = new WebSocket(wsUrl(session.id, csrf, initialCols, initialRows));
		ws.binaryType = "arraybuffer";
		if (!activeSessionIdRef.current || activeSessionIdRef.current === session.id) {
			setConnected(ws.readyState === WebSocket.OPEN);
		}

		const st: SessionTerminal = {
			terminal,
			fitAddon,
			container,
			socket: ws,
			replaying: true,
			queue: [],
			canResize: false,
		};
		terminalsRef.current.set(session.id, st);
		requestAnimationFrame(() => syncTerminalSize(session.id));
		window.setTimeout(() => syncTerminalSize(session.id), 200);
		if (document.fonts?.ready) {
			document.fonts.ready.then(() => syncTerminalSize(session.id)).catch(() => {
				return;
			});
		}

		const handleStream = (bytes: Uint8Array, onDone?: () => void) => {
			if (!bytes || bytes.length === 0) return;
			markActivity(session.id);
			st.terminal.write(bytes, onDone);
		};

		const flushQueue = () => {
			if (st.queue.length === 0) return;
			const queued = [...st.queue];
			st.queue = [];
			queued.forEach((item) => handleStream(item));
		};

		const handleSnapshot = (payload: string) => {
			let snapshotData = payload;
			let cols = 0;
			let rows = 0;
			let fromTmux = false;
			try {
				const parsed = JSON.parse(payload) as {
					type?: string;
					data?: string;
					cols?: number;
					rows?: number;
					tmux?: boolean;
				};
				if (parsed && typeof parsed.data === "string") {
					snapshotData = parsed.data;
				}
				if (typeof parsed?.cols === "number") cols = parsed.cols;
				if (typeof parsed?.rows === "number") rows = parsed.rows;
				if (parsed?.tmux) fromTmux = true;
			} catch (err) {
				void err;
			}

			st.replaying = true;
			st.canResize = false;
			st.terminal.reset();
			if (cols > 0 && rows > 0) {
				try {
					st.terminal.resize(cols, rows);
				} catch (err) {
					void err;
				}
			}
			if (fromTmux) {
				try {
					st.terminal.clear();
				} catch {
					/* ignore */
				}
			}
			const bytes = decodeBase64ToBytes(snapshotData);
			handleStream(bytes, () => {
		st.replaying = false;
		st.canResize = true;
		flushQueue();
		syncTerminalSize(session.id);
		});
		};

		let didForceFit = false;
		ws.addEventListener("message", (ev) => {
			if (typeof ev.data === "string") {
				try {
					const parsed = JSON.parse(ev.data) as { type?: string };
					if (parsed?.type === "snapshot") {
						handleSnapshot(ev.data);
						return;
					}
				} catch (err) {
					void err;
				}
				return;
			}

			const bytes = ev.data instanceof ArrayBuffer ? new Uint8Array(ev.data) : null;
			if (!bytes) return;
			if (st.replaying) {
				st.queue.push(bytes);
				return;
			}
		if (!didForceFit) {
			didForceFit = true;
			syncTerminalSize(session.id);
			window.setTimeout(() => syncTerminalSize(session.id), 120);
		}
		handleStream(bytes);
		});

		ws.addEventListener("open", () => {
			if (activeSessionIdRef.current === session.id && mountedRef.current) {
				setConnected(true);
			}
		syncTerminalSize(session.id);
		window.setTimeout(() => syncTerminalSize(session.id), 160);
		});

		safeTimeout(() => {
			if (st.replaying) {
				st.replaying = false;
				st.canResize = true;
				flushQueue();
				scheduleFit(session.id);
			}
		}, 800);

		const handleSocketClosed = () => {
			if (activeSessionIdRef.current === session.id && mountedRef.current) {
				setConnected(false);
			}
		};
		ws.addEventListener("close", handleSocketClosed);
		ws.addEventListener("error", handleSocketClosed);

		terminal.onData((input: string) => {
			markActivity(session.id);
			void postInput(session.id, input);
		});

      terminal.onResize(({ cols, rows }: { cols: number; rows: number }) => {
        if (!st.canResize) return;
        void postResize(session.id, cols, rows);
      });

		markActivity(session.id);
		setSessions((prev) => (prev.some((s) => s.id === session.id) ? prev : [...prev, session]));

      switchToSession(session.id);
      try {
        terminal.focus();
      } catch {
        /* ignore */
      }

    },
	[
		getCSRFToken,
		loadXterm,
		postInput,
		postResize,
		switchToSession,
		fontSize,
		cursorStyle,
		markActivity,
	],
  );

  const createSession = useCallback(
    async (name?: string) => {
      const visibleCount = sessions.length;
      if (visibleCount >= MAX_SESSIONS) return;
    const parent = activePaneIdRef.current
      ? paneRefs.current.get(activePaneIdRef.current) ?? containerParentRef.current
      : containerParentRef.current;
    if (!parent) return;

      const cols = Math.max(Math.floor(parent.clientWidth / 8), 80);
      const rows = Math.max(Math.floor(parent.clientHeight / 17), 24);
      const sessionName = name ?? `Terminal ${visibleCount + 1}`;

      let res: Response;
      try {
        res = await fetch("/api/terminal/sessions", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          credentials: "include",
          body: JSON.stringify({ name: sessionName, cols, rows }),
        });
      } catch {
        return;
      }
      if (res.status === 401) {
        setAuthenticated(false);
        return;
      }
      if (!res.ok) {
        return;
      }

      const data: { id: string; name: string; csrf_token?: string } = await res.json();
      if (data.csrf_token) csrfRef.current = data.csrf_token;

      const session: SessionInfo = { id: data.id, name: data.name ?? sessionName };
      await attachSession(session);
    },
    [attachSession, sessions],
  );

  const closeSession = useCallback(
    async (id: string) => {
      dropSessionLocal(id);

      try {
        await fetch(`/api/terminal/sessions/${id}`, {
          method: "DELETE",
          credentials: "include",
        });
      } catch (err) {
        void err;
      }
    },
    [dropSessionLocal],
  );

  const handleCopy = useCallback(async () => {
    const id = activeSessionIdRef.current;
    if (!id) {
      pushToast("No active tab", "error");
      return;
    }
    const st = terminalsRef.current.get(id);
    if (!st) {
      pushToast("No active tab", "error");
      return;
    }
    const selection = st.terminal.getSelection();
    if (!selection) {
      pushToast("No selection to copy", "info");
      return;
    }
    try {
      await navigator.clipboard.writeText(selection);
      pushToast("Copied selection", "success");
    } catch {
      pushToast("Copy failed", "error");
    }
  }, [pushToast]);

  const handlePaste = useCallback(async () => {
    const id = activeSessionIdRef.current;
    if (!id) {
      pushToast("No active tab", "error");
      return;
    }
    try {
      const text = await navigator.clipboard.readText();
      if (!text) {
        pushToast("Clipboard empty", "info");
        return;
      }
      await postInput(id, text);
      pushToast("Pasted", "success");
    } catch {
      pushToast("Paste failed", "error");
    }
  }, [postInput, pushToast]);

  const handleSendNotify = useCallback(async () => {
    if (!monitoringActiveId) return;
    if (!monitoringNotifyTitle.trim()) {
      pushToast("Title required", "error");
      return;
    }
    setMonitoringNotifying(true);
    try {
      const res = await fetch("/api/monitoring/v1/notify", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          session_id: monitoringActiveId,
          title: monitoringNotifyTitle,
          message: monitoringNotifyMessage,
          level: "wait",
        }),
      });
      if (!res.ok) {
        pushToast("Notification failed", "error");
        return;
      }
      pushToast("Notification sent", "success");
      setMonitoringNotifyTitle("");
      setMonitoringNotifyMessage("");
    } catch (err) {
      void err;
      pushToast("Notification failed", "error");
    } finally {
      setMonitoringNotifying(false);
    }
  }, [monitoringActiveId, monitoringNotifyTitle, monitoringNotifyMessage, pushToast]);

  /* ---------------------------------------------------------------- */
  /*  Auth                                                             */
  /* ---------------------------------------------------------------- */

  const checkAuth = useCallback(async () => {
    try {
      const res = await fetch("/api/me", { credentials: "include" });
      if (res.ok) {
        const data: { csrf_token?: string; idle_timeout_seconds?: number } = await res.json();
        if (data.csrf_token) csrfRef.current = data.csrf_token;
        if (typeof data.idle_timeout_seconds === "number") {
          setIdleTimeoutSec(data.idle_timeout_seconds);
        }
        setAuthenticated(true);
        return true;
      }
      if (res.status === 401) {
        setAuthenticated(false);
      }
    } catch {
      /* not authed */
    }
    return false;
  }, []);

  const handleLogin = useCallback(async () => {
    setLoggingIn(true);
    setLoginError("");
    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
		body: JSON.stringify({ password }),
      });
      if (res.ok) {
        const data: { csrf_token?: string } = await res.json();
        if (data.csrf_token) csrfRef.current = data.csrf_token;
        setAuthenticated(true);
        setPassword("");
      } else {
        setLoginError("Invalid password");
      }
    } catch {
      setLoginError("Connection failed");
    } finally {
      setLoggingIn(false);
    }
  }, [password]);

  const initializeSessions = useCallback(async () => {
    let res: Response;
    try {
      res = await fetch("/api/terminal/sessions", { credentials: "include" });
    } catch {
      await createSession("Terminal 1");
      return;
    }

    if (res.status === 401) {
      setAuthenticated(false);
      return;
    }
    if (!res.ok) {
      await res.text();
      await createSession("Terminal 1");
      return;
    }

    const data: { sessions?: Array<{ id: string; name: string }> } = await res.json();
    const existing = Array.isArray(data.sessions) ? data.sessions : [];
    if (existing.length === 0) {
      await createSession("Terminal 1");
      return;
    }

    const list = isDetached && detachedSessionId
      ? existing.filter((s) => s.id === detachedSessionId)
      : existing;

    for (const s of list) {
      await attachSession({ id: s.id, name: s.name });
    }
  }, [attachSession, createSession, isDetached, detachedSessionId]);

  /* ---------------------------------------------------------------- */
  /*  Mount / unmount                                                   */
  /* ---------------------------------------------------------------- */

  useEffect(() => {
    mountedRef.current = true;

    checkAuth().catch(() => {
      return;
    });

    return () => {
      mountedRef.current = false;
		terminalsRef.current.forEach((st) => {
			try {
				st.socket.close();
			} catch (err) {
				void err;
			}
        st.terminal.dispose();
        st.container.parentNode?.removeChild(st.container);
      });
      terminalsRef.current.clear();
      timersRef.current.forEach((id) => clearTimeout(id));
      timersRef.current.clear();
    };
  }, [checkAuth]);

  const didInitSessions = useRef(false);
  useEffect(() => {
    if (!authenticated || didInitSessions.current) return;
    didInitSessions.current = true;
    void initializeSessions();
  }, [authenticated, initializeSessions]);

  useEffect(() => {
    if (!isDetached || !detachedSessionId) return;
    const exists = sessions.some((s) => s.id === detachedSessionId);
    if (exists) {
      switchToSession(detachedSessionId);
    }
  }, [detachedSessionId, isDetached, sessions, switchToSession]);

  useEffect(() => {
    if (!authenticated) return;
    let cancelled = false;

    const check = async () => {
      try {
        const res = await fetch("/api/health", { credentials: "include" });
        if (cancelled) return;
        setServerAlive(res.ok);
      } catch (err) {
        void err;
        if (!cancelled) setServerAlive(false);
      }
    };

    void check();
    const id = window.setInterval(check, 8000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [authenticated]);


  useEffect(() => {
    if (!monitoringOpen) return;
    let cancelled = false;

    const loadSummary = async () => {
      try {
        const res = await fetch("/api/monitoring/v1/summary", { credentials: "include" });
        if (!res.ok) return;
        const data = (await res.json()) as { sessions: MonitoringSessionSummary[] };
        if (cancelled) return;
        const normalized = (data.sessions ?? []).map((session) => ({
          ...session,
          activity: session.activity ?? [],
        }));
        setMonitoringSessions(normalized);
        if (!monitoringActiveId && data.sessions?.length) {
          setMonitoringActiveId(data.sessions[0].id);
        }
      } catch (err) {
        void err;
      }
    };

    void loadSummary();
    const es = new EventSource("/api/monitoring/v1/stream", { withCredentials: true });
    monitoringStreamRef.current = es;
    es.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data) as { type: string; payload: any };
        if (msg.type === "snapshot" || msg.type === "update") {
          const sessions = msg.payload?.sessions as MonitoringSessionSummary[] | undefined;
          if (sessions && !cancelled) {
            const normalized = sessions.map((session) => ({
              ...session,
              activity: session.activity ?? [],
            }));
            setMonitoringSessions(normalized);
            if (!monitoringActiveId && sessions.length) {
              setMonitoringActiveId(sessions[0].id);
            }
          }
        }
        if (msg.type === "event") {
          const evt = msg.payload as MonitoringEvent;
          if (!evt?.session_id) return;
          setMonitoringEvents((prev) => {
            const list = prev[evt.session_id] ?? [];
            return { ...prev, [evt.session_id]: [evt, ...list].slice(0, 50) };
          });
        }
      } catch (err) {
        void err;
      }
    };
    es.onerror = () => {
      if (monitoringStreamRef.current) {
        monitoringStreamRef.current.close();
      }
    };

    return () => {
      cancelled = true;
      if (monitoringStreamRef.current) {
        monitoringStreamRef.current.close();
      }
    };
  }, [monitoringOpen, monitoringActiveId]);

  useEffect(() => {
    if (!monitoringActiveId) return;
    const loadEvents = async () => {
      try {
        const res = await fetch(`/api/monitoring/v1/events?session_id=${monitoringActiveId}`, {
          credentials: "include",
        });
        if (!res.ok) return;
        const data = (await res.json()) as { events: MonitoringEvent[] };
        setMonitoringEvents((prev) => ({ ...prev, [monitoringActiveId]: data.events ?? [] }));
      } catch (err) {
        void err;
      }
    };

    void loadEvents();
  }, [monitoringActiveId]);

  useEffect(() => {
    if (!monitoringActiveId) return;
    setMonitoringNotifyTitle("");
    setMonitoringNotifyMessage("");
  }, [monitoringActiveId]);

  useEffect(() => {
    if (!activeId) return;
    if (!layoutBySession[activeId]) {
      const pane = createPaneNode(activeId, `pane-${Date.now()}`);
      setLayoutBySession((prev) => ({ ...prev, [activeId]: pane }));
      setActivePaneBySession((prev) => ({ ...prev, [activeId]: pane.id }));
      return;
    }
    if (!activePaneBySession[activeId]) {
      const firstPaneId = getFirstPaneId(layoutBySession[activeId]);
      setActivePaneBySession((prev) => ({ ...prev, [activeId]: firstPaneId }));
    }
  }, [activeId, layoutBySession, activePaneBySession]);

	useEffect(() => {
		if (!activePaneSessionId) {
			setConnected(false);
			return;
		}
		const st = terminalsRef.current.get(activePaneSessionId);
		if (!st) return;
		setConnected(st.socket.readyState === WebSocket.OPEN);
		scheduleFit(activePaneSessionId);
	}, [activePaneSessionId, scheduleFit]);

	useEffect(() => {
		if (!activeId) return;
		const panes = activePanes;
		panes.forEach((pane) => {
			const st = terminalsRef.current.get(pane.sessionId);
			const host = paneRefs.current.get(pane.id);
			if (!st || !host) return;
			if (st.container.parentNode !== host) {
				host.appendChild(st.container);
			}
			st.container.style.visibility = "visible";
			st.container.style.zIndex = "1";
			st.canResize = pane.sessionId === activePaneSessionId;
		});
    terminalsRef.current.forEach((st, id) => {
      if (!panes.some((p) => p.sessionId === id)) {
        st.container.style.visibility = "hidden";
        if (containerParentRef.current && st.container.parentNode !== containerParentRef.current) {
          containerParentRef.current.appendChild(st.container);
        }
      }
    });
		requestAnimationFrame(() => {
			if (!activeId) return;
			scheduleFit(activeId);
		});
	}, [activeId, activePanes, activePaneSessionId, scheduleFit]);

	useEffect(() => {
		if (!activeId || !activePaneId) return;
		const host = paneRefs.current.get(activePaneId) ?? null;
		activeResizeObserverRef.current?.disconnect();
		if (!host) {
			safeTimeout(() => {
				if (activeIdRef.current === activeId) scheduleFit(activeId);
			}, 120);
			return;
		}
		const observer = new ResizeObserver(() => {
			scheduleFit(activeId);
		});
		observer.observe(host);
		activeResizeObserverRef.current = observer;
		scheduleFit(activeId);
		return () => observer.disconnect();
	}, [activeId, activePaneId, safeTimeout, scheduleFit]);

  useEffect(() => {
    if (!authenticated) return;
    if (idleTimeoutSec <= 0) return;
    const warnAtMs = Math.max(0, idleTimeoutSec * 1000 - 5 * 60 * 1000);
    const id = window.setInterval(() => {
      const activeId = activeSessionIdRef.current;
      if (!activeId) return;
      const last = lastActivityRef.current.get(activeId);
      if (!last) return;
      const idleFor = Date.now() - last;
      if (idleFor >= warnAtMs && !idleWarnedRef.current.has(activeId)) {
        idleWarnedRef.current.add(activeId);
        const remaining = Math.max(0, idleTimeoutSec * 1000 - idleFor);
        const minutes = Math.max(1, Math.ceil(remaining / 60000));
        pushToast(`Idle timeout in ${minutes} min`, "info");
      }
    }, 30000);
    return () => window.clearInterval(id);
  }, [authenticated, idleTimeoutSec, pushToast]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const updateViewport = () => {
      const vv = window.visualViewport;
      const height = window.innerHeight;
      let keyboardOffset = 0;
      if (vv) {
        keyboardOffset = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
      }
      document.documentElement.style.setProperty("--app-height", `${height}px`);
      document.documentElement.style.setProperty("--keyboard-offset", `${keyboardOffset}px`);
      if (activeIdRef.current) scheduleFit(activeIdRef.current);
    };

    updateViewport();
    window.visualViewport?.addEventListener("resize", updateViewport);
    window.visualViewport?.addEventListener("scroll", updateViewport);
    window.addEventListener("orientationchange", updateViewport);
    window.addEventListener("resize", updateViewport);
    return () => {
      window.visualViewport?.removeEventListener("resize", updateViewport);
      window.visualViewport?.removeEventListener("scroll", updateViewport);
      window.removeEventListener("orientationchange", updateViewport);
      window.removeEventListener("resize", updateViewport);
    };
  }, [scheduleFit]);

	useEffect(() => {
		terminalsRef.current.forEach((st) => {
			st.terminal.options.fontSize = fontSize;
			st.terminal.options.cursorStyle = cursorStyle;
		});
		window.localStorage.setItem("webterm-font-size", String(fontSize));
		window.localStorage.setItem("webterm-cursor-style", cursorStyle);
		const activeId = activeSessionIdRef.current;
		if (activeId) scheduleFit(activeId);
	}, [fontSize, cursorStyle, scheduleFit]);

  /* ---------------------------------------------------------------- */
  /*  Global events                                                    */
  /* ---------------------------------------------------------------- */

	useEffect(() => {
		const handleFullscreenChange = () => {
			const fs = !!document.fullscreenElement;
			setIsFullscreen(fs);
			if (activeIdRef.current) {
				scheduleFit(activeIdRef.current);
				window.setTimeout(() => scheduleFit(activeIdRef.current as string), 140);
			}
		};

    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => {
      document.removeEventListener("fullscreenchange", handleFullscreenChange);
    };
	}, [scheduleFit]);

	useEffect(() => {
		const handleVisibility = () => {
			const id = activeSessionIdRef.current;
			if (!id) return;
			if (document.hidden) {
				sendFocusEvent(id, false);
				return;
			}
			scheduleFit(id);
			const st = terminalsRef.current.get(id);
			if (st) {
				try {
					st.terminal.focus();
				} catch {
					/* ignore */
				}
			}
			sendFocusEvent(id, true);
		};

		const handleWindowFocus = () => {
			const id = activeSessionIdRef.current;
			if (!id || document.hidden) return;
			scheduleFit(id);
			const st = terminalsRef.current.get(id);
			if (st) {
				try {
					st.terminal.focus();
				} catch {
					/* ignore */
				}
			}
			sendFocusEvent(id, true);
		};

		const handleWindowBlur = () => {
			const id = activeSessionIdRef.current;
			if (!id) return;
			sendFocusEvent(id, false);
		};

		document.addEventListener("visibilitychange", handleVisibility);
		window.addEventListener("focus", handleWindowFocus);
		window.addEventListener("blur", handleWindowBlur);
		return () => {
			document.removeEventListener("visibilitychange", handleVisibility);
			window.removeEventListener("focus", handleWindowFocus);
			window.removeEventListener("blur", handleWindowBlur);
		};
	}, [scheduleFit, sendFocusEvent]);

  /* ---------------------------------------------------------------- */
  /*  Fullscreen                                                       */
  /* ---------------------------------------------------------------- */

  const workspaceRef = useRef<HTMLDivElement | null>(null);

  const toggleFullscreen = useCallback(() => {
    if (!workspaceRef.current) return;
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    } else {
      workspaceRef.current.requestFullscreen().catch(() => {});
    }
  }, []);

  /* ---------------------------------------------------------------- */
  /*  Active session helper                                            */
  /* ---------------------------------------------------------------- */

  const activeSession = sessions.find((s) => s.id === activeId);
  const showConnected = connected && serverAlive;
  const activeForActions = sessions.find((s) => s.id === tabActionsId) ?? null;
  const monitoringEventList = monitoringActiveId ? monitoringEvents[monitoringActiveId] ?? [] : [];
  const orderedSessions = useMemo(() => sessions, [sessions]);
  const visibleSessions = useMemo(() => {
    if (isDetached && detachedSessionId) {
      return orderedSessions.filter((s) => s.id === detachedSessionId);
    }
    return orderedSessions;
  }, [detachedSessionId, isDetached, orderedSessions]);
  const canCreateSession = visibleSessions.length < MAX_SESSIONS;

  /* keyboard shortcuts */
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.code === "KeyK") {
        e.preventDefault();
        const target = e.target as HTMLElement | null;
        if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA")) return;
        setQuickSwitchOpen(true);
        return;
      }
      if (!e.altKey || !e.shiftKey) return;

      if (e.code === "KeyN") {
        e.preventDefault();
        if (!canCreateSession) {
          pushToast("Maximum of 5 terminals reached", "info");
          return;
        }
        createSession();
        return;
      }

      if (e.code === "KeyF") {
        e.preventDefault();
        toggleFullscreen();
        return;
      }

      if (e.code === "ArrowLeft" || e.code === "ArrowRight") {
        e.preventDefault();
        if (visibleSessions.length < 2) return;
        const idx = visibleSessions.findIndex((s) => s.id === activeIdRef.current);
        if (idx === -1) return;
        const next =
          e.code === "ArrowLeft"
            ? (idx - 1 + visibleSessions.length) % visibleSessions.length
            : (idx + 1) % visibleSessions.length;
        switchToSession(visibleSessions[next].id);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visibleSessions, createSession, switchToSession, toggleFullscreen, canCreateSession, pushToast]);

  /* ================================================================ */
  /*  LOGIN SCREEN                                                     */
  /* ================================================================ */

  if (!authenticated) {
		return (
			<LoginScreen
				password={password}
				loginError={loginError}
				loggingIn={loggingIn}
				onPasswordChange={setPassword}
				onSubmit={handleLogin}
			/>
		);
	}

  /* ================================================================ */
  /*  TERMINAL WORKSPACE                                               */
  /* ================================================================ */

  return (
    <div
      ref={workspaceRef}
      className={cn(
        "app-root relative flex w-screen flex-col bg-[radial-gradient(circle_at_top_left,_var(--app-bg-highlight)_0%,_var(--app-bg)_55%,_var(--app-bg)_100%)] text-[var(--app-text)]",
        isFullscreen && "fixed inset-0 z-50",
      )}
    >
		<HeaderBar
			showConnected={showConnected}
			isFullscreen={isFullscreen}
			onMonitoringOpen={() => setMonitoringOpen(true)}
		/>
      <MobileStatusStrip showConnected={showConnected} isFullscreen={isFullscreen} />

      {/* ---- main ---- */}
      <div className={cn("flex min-h-0 w-full flex-1 flex-col pb-8", isFullscreen && "h-screen")}>
        <TabsRow
          sessions={visibleSessions}
          activeId={activeId}
          dragId={dragId}
          canCreateSession={canCreateSession}
          isFullscreen={isFullscreen}
          renamingId={renamingId}
          renamingValue={renamingValue}
          onCreateSession={() => {
            if (!canCreateSession) {
              pushToast("Maximum of 5 terminals reached", "info");
              return;
            }
            createSession();
          }}
          onToggleFullscreen={toggleFullscreen}
          onSwitchSession={(id) => {
            switchToSession(id);
          }}
          onDetachSession={(id) => {
            dropSessionLocal(id);
            openDetached(id);
          }}
          onCloseSession={closeSession}
          onSetDragId={setDragId}
          onMoveSession={moveSession}
          onOpenTabActions={setTabActionsId}
          onTabHoldStart={(id) => {
            const timer = setTimeout(() => setTabActionsId(id), 550);
            tabHoldTimersRef.current.set(id, timer);
          }}
          onTabHoldEnd={(id) => {
            const timer = tabHoldTimersRef.current.get(id);
            if (timer) window.clearTimeout(timer);
            tabHoldTimersRef.current.delete(id);
          }}
          onRenameStart={(id) => {
            const session = sessions.find((s) => s.id === id);
            if (!session) return;
            setRenamingId(id);
            setRenamingValue(session.name);
          }}
          onRenameChange={setRenamingValue}
          onRenameSubmit={async () => {
            if (!renamingId) return;
            const nextName = renamingValue.trim();
            if (!nextName) {
              pushToast("Name required", "error");
              return;
            }
            await renameSession(renamingId, nextName);
            setRenamingId(null);
            setRenamingValue("");
          }}
          onRenameCancel={() => {
            setRenamingId(null);
            setRenamingValue("");
          }}
        />
        {/* ---- terminal area ---- */}
        <TerminalStack
          layout={activeLayout}
          activePaneId={activePaneId}
          isFullscreen={isFullscreen}
			onActivatePane={(id) => {
				if (!activeId) return;
				setActivePaneBySession((prev) => ({ ...prev, [activeId]: id }));
				if (!activeLayout) return;
				const pane = findPaneById(activeLayout, id);
				if (!pane) return;
				scheduleFit(pane.sessionId);
			}}
          registerPaneRef={(id, node) => {
            if (node) {
              paneRefs.current.set(id, node);
            } else {
              paneRefs.current.delete(id);
            }
          }}
          containerParentRef={containerParentRef}
        />
      </div>
      <StatusBar
        showConnected={showConnected}
        activeSession={activeSession ?? null}
        onOpenSettings={() => setSettingsOpen(true)}
        onCopy={handleCopy}
        onPaste={handlePaste}
      />

      <SettingsModal
        open={settingsOpen}
        fontSize={fontSize}
        cursorStyle={cursorStyle}
        cursorStyles={CURSOR_STYLES}
        onClose={() => setSettingsOpen(false)}
        onFontSizeChange={setFontSize}
        onCursorStyleChange={(value) => setCursorStyle(value as (typeof CURSOR_STYLES)[number])}
      />


      <MonitoringModal
        open={monitoringOpen}
        sessions={monitoringSessions}
        selectedId={monitoringActiveId}
        events={monitoringEventList}
        notifyTitle={monitoringNotifyTitle}
        notifyMessage={monitoringNotifyMessage}
        notifying={monitoringNotifying}
        onClose={() => setMonitoringOpen(false)}
        onSelect={(id) => setMonitoringActiveId(id)}
        onNotifyTitleChange={setMonitoringNotifyTitle}
        onNotifyMessageChange={setMonitoringNotifyMessage}
        onSendNotify={handleSendNotify}
      />

      <TabActionsSheet
        session={activeForActions}
        onClose={() => setTabActionsId(null)}
        onSwitch={(id) => {
          switchToSession(id);
          setTabActionsId(null);
        }}
        onRename={(id) => {
          const session = sessions.find((s) => s.id === id);
          if (session) {
            setRenamingId(id);
            setRenamingValue(session.name);
          }
          setTabActionsId(null);
        }}
        onDetach={(id) => {
          dropSessionLocal(id);
          openDetached(id);
          setTabActionsId(null);
        }}
        onCloseSession={(id) => {
          closeSession(id);
          setTabActionsId(null);
        }}
      />
      <QuickSwitcher
        open={quickSwitchOpen}
        query={quickQuery}
        sessions={visibleSessions}
        activeId={activeId}
        onClose={() => {
          setQuickSwitchOpen(false);
          setQuickQuery("");
        }}
        onQueryChange={setQuickQuery}
        onSelect={(id) => {
          switchToSession(id);
          setQuickSwitchOpen(false);
          setQuickQuery("");
        }}
      />
      <ToastStack toasts={toasts} />
    </div>
  );
}
