"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";

import { HeaderBar } from "@/components/terminal/HeaderBar";
import { LoginScreen } from "@/components/terminal/LoginScreen";
import { MetricsModal } from "@/components/terminal/MetricsModal";
import { MobileStatusStrip } from "@/components/terminal/MobileStatusStrip";
import { QuickSwitcher } from "@/components/terminal/QuickSwitcher";
import { SettingsModal } from "@/components/terminal/SettingsModal";
import { StatusBar } from "@/components/terminal/StatusBar";
import { TabsRow } from "@/components/terminal/TabsRow";
import { TabActionsSheet } from "@/components/terminal/TabActionsSheet";
import { TerminalStack } from "@/components/terminal/TerminalStack";
import { ToastStack, ToastItem } from "@/components/terminal/ToastStack";
import { MetricsSnapshot, SessionInfo } from "@/components/terminal/types";

import type { Terminal } from "xterm";
import type { FitAddon } from "xterm-addon-fit";

interface SessionTerminal {
	terminal: Terminal;
	fitAddon: FitAddon;
	container: HTMLDivElement;
	eventSource: EventSource;
	observer: ResizeObserver;
	replaying: boolean;
	queue: string[];
}

const TERM_OPTIONS = {
  cursorBlink: true,
  scrollback: 5000,
  fontSize: 13,
  fontFamily:
    "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, Courier New, monospace",
} as const;

const TERM_THEMES = {
  dark: {
    background: "#030712",
    foreground: "#dbe5f5",
    cursor: "#7dd3fc",
  },
  light: {
    background: "#f8fafc",
    foreground: "#0f172a",
    cursor: "#0284c7",
  },
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
  const raw = atob(data);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i += 1) {
    bytes[i] = raw.charCodeAt(i);
  }
  return bytes;
}

function sseUrl(sessionId: string, csrf: string): string {
	return `${window.location.origin}/api/terminal/stream/${sessionId}?csrf=${encodeURIComponent(csrf)}`;
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
  const [theme, setTheme] = useState<"dark" | "light">("dark");
  const [fontSize, setFontSize] = useState(13);
  const [cursorStyle, setCursorStyle] = useState<(typeof CURSOR_STYLES)[number]>("block");
  const [dragId, setDragId] = useState<string | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [metrics, setMetrics] = useState<MetricsSnapshot | null>(null);
  const [metricsOpen, setMetricsOpen] = useState<"cpu" | "memory" | "gpu" | null>(null);
  const [metricsPage, setMetricsPage] = useState(0);
  const [detachedSessionId, setDetachedSessionId] = useState<string | null>(null);
  const [isDetached, setIsDetached] = useState(false);
  const [tabActionsId, setTabActionsId] = useState<string | null>(null);
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renamingValue, setRenamingValue] = useState("");
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [quickSwitchOpen, setQuickSwitchOpen] = useState(false);
  const [quickQuery, setQuickQuery] = useState("");
  const [idleTimeoutSec, setIdleTimeoutSec] = useState(0);

  /* ---- refs ---- */
  const csrfRef = useRef("");
  const terminalsRef = useRef<Map<string, SessionTerminal>>(new Map());
  const containerParentRef = useRef<HTMLDivElement | null>(null);
  const activeIdRef = useRef<string | null>(null);
  const timersRef = useRef<Set<ReturnType<typeof setTimeout>>>(new Set());
  const tabHoldTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const mountedRef = useRef(true);
  const lastActivityRef = useRef<Map<string, number>>(new Map());
  const idleWarnedRef = useRef<Set<string>>(new Set());

  /* keep activeIdRef in sync */
  useEffect(() => {
    activeIdRef.current = activeId;
  }, [activeId]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const sessionId = params.get("session");
    const detached = params.get("detached") === "1" && sessionId;
    setDetachedSessionId(sessionId);
    setIsDetached(!!detached);
  }, []);

  useEffect(() => {
    const stored = window.localStorage.getItem("webterm-theme");
    if (stored === "light" || stored === "dark") {
      setTheme(stored);
      document.documentElement.dataset.theme = stored;
      return;
    }
    const prefersLight = window.matchMedia("(prefers-color-scheme: light)").matches;
    const initial = prefersLight ? "light" : "dark";
    setTheme(initial);
    document.documentElement.dataset.theme = initial;
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

  const xtermModRef = useRef<{ Terminal: typeof Terminal; FitAddon: typeof FitAddon } | null>(null);

  const loadXterm = useCallback(async () => {
    if (xtermModRef.current) return xtermModRef.current;
    const [xtermMod, fitMod] = await Promise.all([
      import("xterm"),
      import("xterm-addon-fit"),
    ]);
    xtermModRef.current = {
      Terminal: xtermMod.Terminal,
      FitAddon: fitMod.FitAddon,
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

  const toggleTheme = useCallback(() => {
    setTheme((prev) => {
      const next = prev === "dark" ? "light" : "dark";
      document.documentElement.dataset.theme = next;
      window.localStorage.setItem("webterm-theme", next);
      return next;
    });
  }, []);

  const getCSRFToken = useCallback(() => {
    return csrfRef.current || getCookie("webterm_csrf");
  }, []);

  const markActivity = useCallback((id: string) => {
    lastActivityRef.current.set(id, Date.now());
    idleWarnedRef.current.delete(id);
  }, []);

  const postInput = useCallback(
    async (id: string, input: string) => {
      const csrf = getCSRFToken();
      if (!csrf || !input) return;
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
    [getCSRFToken],
  );

  const postResize = useCallback(
    async (id: string, cols: number, rows: number) => {
      const csrf = getCSRFToken();
      if (!csrf) return;
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
    [getCSRFToken],
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

  const fitActive = useCallback(() => {
    const id = activeIdRef.current;
    if (!id) return;
    const st = terminalsRef.current.get(id);
    if (!st) return;
    try {
      st.fitAddon.fit();
    } catch {
      /* container may not be visible yet */
    }
  }, []);

  const deferredFitActive = useCallback(() => {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        safeTimeout(fitActive, 120);
      });
    });
  }, [fitActive, safeTimeout]);

  const switchToSession = useCallback(
    (id: string) => {
      const map = terminalsRef.current;
      /* hide all */
      map.forEach((st, sid) => {
        if (sid === id) {
          st.container.style.zIndex = "1";
          st.container.style.visibility = "visible";
        } else {
          st.container.style.zIndex = "0";
          st.container.style.visibility = "hidden";
        }
      });
      setActiveId(id);
      const st = map.get(id);
      if (st) {
        setConnected(st.eventSource.readyState !== EventSource.CLOSED);
        requestAnimationFrame(() => {
          try {
            st.fitAddon.fit();
          } catch {
            /* ignore */
          }
          st.terminal.focus();
        });
      }
    },
    [],
  );

  const dropSessionLocal = useCallback(
    (id: string) => {
      const st = terminalsRef.current.get(id);
      if (st) {
        try {
          st.eventSource.close();
        } catch (err) {
          void err;
        }
        st.observer.disconnect();
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
        theme: TERM_THEMES[theme],
      });
      const fitAddon = new mods.FitAddon();
      terminal.loadAddon(fitAddon);

      const container = document.createElement("div");
      container.className = "terminal-shell";
      container.style.position = "absolute";
      container.style.inset = "0";
      container.style.zIndex = "0";
      container.style.visibility = "hidden";
      parent.appendChild(container);

      terminal.open(container);

      const observer = new ResizeObserver(() => {
        if (activeIdRef.current === session.id) {
          requestAnimationFrame(() => {
            try {
              fitAddon.fit();
            } catch (err) {
              void err;
            }
          });
        }
      });
      observer.observe(container);

      const csrf = getCSRFToken();
      const eventSource = new EventSource(sseUrl(session.id, csrf), { withCredentials: true });
      if (!activeIdRef.current || activeIdRef.current === session.id) {
        setConnected(eventSource.readyState !== EventSource.CLOSED);
      }

      const st: SessionTerminal = {
        terminal,
        fitAddon,
        container,
        eventSource,
        observer,
        replaying: true,
        queue: [],
      };

      const handleStream = (data: string, onDone?: () => void) => {
        if (!data) return;
        const bytes = decodeBase64ToBytes(data);
        markActivity(session.id);
        st.terminal.write(bytes, onDone);
      };

      const flushQueue = () => {
        if (st.queue.length === 0) return;
        const queued = [...st.queue];
        st.queue = [];
        queued.forEach((item) => handleStream(item));
      };

      eventSource.addEventListener("snapshot", (ev: MessageEvent<string>) => {
        let snapshotData = ev.data;
        let cols = 0;
        let rows = 0;
        try {
          const parsed = JSON.parse(ev.data) as {
            data?: string;
            cols?: number;
            rows?: number;
          };
          if (parsed && typeof parsed.data === "string") {
            snapshotData = parsed.data;
          }
          if (typeof parsed?.cols === "number") cols = parsed.cols;
          if (typeof parsed?.rows === "number") rows = parsed.rows;
        } catch (err) {
          void err;
        }

        st.replaying = true;
        st.terminal.reset();
        if (cols > 0 && rows > 0) {
          try {
            st.terminal.resize(cols, rows);
          } catch (err) {
            void err;
          }
        }
        handleStream(snapshotData, () => {
          st.replaying = false;
          flushQueue();
          try {
            st.fitAddon.fit();
          } catch (err) {
            void err;
          }
        });
      });

      eventSource.addEventListener("message", (ev: MessageEvent<string>) => {
        if (st.replaying) {
          st.queue.push(ev.data);
          return;
        }
        handleStream(ev.data);
      });

      eventSource.addEventListener("open", () => {
        if (activeIdRef.current === session.id && mountedRef.current) {
          setConnected(true);
        }
      });

      safeTimeout(() => {
        if (st.replaying) {
          st.replaying = false;
          flushQueue();
        }
      }, 800);

      eventSource.addEventListener("error", () => {
        if (activeIdRef.current === session.id && mountedRef.current) {
          setConnected(false);
        }
      });

      terminal.onData((input: string) => {
        const isMouse = input.startsWith("\u001b[M") || input.startsWith("\u001b[<");
        const isFocus = input === "\u001b[I" || input === "\u001b[O";
        if (isMouse || isFocus) {
          const bufferType = st.terminal.buffer?.active?.type;
          const modes = (terminal as Terminal & { modes?: { mouseTracking?: boolean; focus?: boolean } }).modes;
          const allowMouse = bufferType === "alternate" && modes?.mouseTracking;
          const allowFocus = bufferType === "alternate" && modes?.focus;
          if (isMouse && !allowMouse) return;
          if (isFocus && !allowFocus) return;
        }
        markActivity(session.id);
        void postInput(session.id, input);
      });

      terminal.onResize(({ cols, rows }: { cols: number; rows: number }) => {
        void postResize(session.id, cols, rows);
      });

      terminalsRef.current.set(session.id, st);
      markActivity(session.id);
      setSessions((prev) => (prev.some((s) => s.id === session.id) ? prev : [...prev, session]));
      switchToSession(session.id);

      try {
        fitAddon.fit();
      } catch (err) {
        void err;
      }
    },
    [getCSRFToken, loadXterm, postInput, postResize, switchToSession, theme, fontSize, cursorStyle],
  );

  const createSession = useCallback(
    async (name?: string) => {
      if (sessions.length >= MAX_SESSIONS) return;
      const parent = containerParentRef.current;
      if (!parent) return;

      const cols = Math.max(Math.floor(parent.clientWidth / 8), 80);
      const rows = Math.max(Math.floor(parent.clientHeight / 17), 24);
      const sessionName = name ?? `Terminal ${sessions.length + 1}`;

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
    [attachSession, sessions.length],
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
    const id = activeIdRef.current;
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
    const id = activeIdRef.current;
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
          st.eventSource.close();
        } catch (err) {
          void err;
        }
        st.observer.disconnect();
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
    if (!authenticated) return;
    let cancelled = false;

    const loadMetrics = async () => {
      try {
        const res = await fetch(`/api/metrics?limit=10&offset=${metricsPage * 10}`, { credentials: "include" });
        if (!res.ok) return;
        const data = (await res.json()) as MetricsSnapshot;
        if (!cancelled) setMetrics(data);
      } catch (err) {
        void err;
      }
    };

    void loadMetrics();
    const id = window.setInterval(loadMetrics, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [authenticated, metricsPage]);

  useEffect(() => {
    if (!authenticated) return;
    if (idleTimeoutSec <= 0) return;
    const warnAtMs = Math.max(0, idleTimeoutSec * 1000 - 5 * 60 * 1000);
    const id = window.setInterval(() => {
      const activeId = activeIdRef.current;
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
      deferredFitActive();
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
  }, [deferredFitActive]);

  useEffect(() => {
    terminalsRef.current.forEach((st) => {
      st.terminal.options.theme = TERM_THEMES[theme];
      st.terminal.refresh(0, st.terminal.rows - 1);
    });
  }, [theme]);

  useEffect(() => {
    terminalsRef.current.forEach((st) => {
      st.terminal.options.fontSize = fontSize;
      st.terminal.options.cursorStyle = cursorStyle;
      try {
        st.fitAddon.fit();
      } catch (err) {
        void err;
      }
    });
    window.localStorage.setItem("webterm-font-size", String(fontSize));
    window.localStorage.setItem("webterm-cursor-style", cursorStyle);
  }, [fontSize, cursorStyle]);

  /* ---------------------------------------------------------------- */
  /*  Global events                                                    */
  /* ---------------------------------------------------------------- */

  useEffect(() => {
    const handleVisibility = () => {
      if (document.visibilityState === "visible") {
        const id = activeIdRef.current;
        if (!id) return;
        const st = terminalsRef.current.get(id);
        if (!st) return;
        try {
          st.fitAddon.fit();
          st.terminal.refresh(0, st.terminal.rows - 1);
        } catch {
          /* ignore */
        }
      }
    };

    const handleFullscreenChange = () => {
      const fs = !!document.fullscreenElement;
      setIsFullscreen(fs);
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          safeTimeout(fitActive, 150);
        });
      });
    };

    const handleResize = () => {
      deferredFitActive();
    };

    document.addEventListener("visibilitychange", handleVisibility);
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    window.addEventListener("resize", handleResize);

    return () => {
      document.removeEventListener("visibilitychange", handleVisibility);
      document.removeEventListener("fullscreenchange", handleFullscreenChange);
      window.removeEventListener("resize", handleResize);
    };
  }, [fitActive, deferredFitActive, safeTimeout]);

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
  const canCreateSession = sessions.length < MAX_SESSIONS;
  const activeForActions = sessions.find((s) => s.id === tabActionsId) ?? null;
  const orderedSessions = useMemo(() => sessions, [sessions]);
  const visibleSessions = useMemo(() => {
    if (isDetached && detachedSessionId) {
      return orderedSessions.filter((s) => s.id === detachedSessionId);
    }
    return orderedSessions;
  }, [detachedSessionId, isDetached, orderedSessions]);

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
        if (sessions.length < 2) return;
        const idx = sessions.findIndex((s) => s.id === activeIdRef.current);
        if (idx === -1) return;
        const next =
          e.code === "ArrowLeft"
            ? (idx - 1 + sessions.length) % sessions.length
            : (idx + 1) % sessions.length;
        switchToSession(sessions[next].id);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessions, createSession, switchToSession, toggleFullscreen, canCreateSession, pushToast]);

  /* ================================================================ */
  /*  LOGIN SCREEN                                                     */
  /* ================================================================ */

  if (!authenticated) {
    return (
      <LoginScreen
        password={password}
        loginError={loginError}
        loggingIn={loggingIn}
        theme={theme}
        onPasswordChange={setPassword}
        onSubmit={handleLogin}
        onThemeToggle={toggleTheme}
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
        theme={theme}
        metrics={metrics}
        showConnected={showConnected}
        isFullscreen={isFullscreen}
        onThemeToggle={toggleTheme}
        onMetricsOpen={setMetricsOpen}
      />
      <MobileStatusStrip metrics={metrics} showConnected={showConnected} isFullscreen={isFullscreen} />

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
          onSwitchSession={switchToSession}
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
          sessions={sessions}
          isFullscreen={isFullscreen}
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
        theme={theme}
        onClose={() => setSettingsOpen(false)}
        onFontSizeChange={setFontSize}
        onCursorStyleChange={(value) => setCursorStyle(value as (typeof CURSOR_STYLES)[number])}
        onThemeChange={setTheme}
      />

      <MetricsModal
        open={metricsOpen}
        metrics={metrics}
        metricsPage={metricsPage}
        onClose={() => setMetricsOpen(null)}
        onPageChange={setMetricsPage}
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
