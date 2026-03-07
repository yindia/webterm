"use client";

import { Moon, Sun, Terminal as TerminalIcon } from "lucide-react";

import { Button } from "@/components/ui/button";

interface LoginScreenProps {
  password: string;
  loginError: string;
  loggingIn: boolean;
  theme: "dark" | "light";
  onPasswordChange: (value: string) => void;
  onSubmit: () => void;
  onThemeToggle: () => void;
}

export function LoginScreen({
  password,
  loginError,
  loggingIn,
  theme,
  onPasswordChange,
  onSubmit,
  onThemeToggle,
}: LoginScreenProps) {
  return (
    <div className="flex h-screen w-screen items-center justify-center bg-[radial-gradient(circle_at_top,_var(--app-bg-highlight)_0%,_var(--app-bg)_55%,_var(--app-bg)_100%)] px-4">
      <div className="w-full max-w-sm rounded-2xl border border-[var(--app-border)] bg-[var(--app-surface)] p-8 shadow-[0_30px_70px_rgba(0,0,0,0.25)] backdrop-blur">
        <div className="mb-6 flex items-center justify-between">
          <div className="flex items-center gap-2 text-[var(--app-accent)]">
            <TerminalIcon className="h-6 w-6" />
            <h1 className="text-xl font-semibold tracking-tight">webterm</h1>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onThemeToggle}
              className="rounded-full border border-[var(--app-border)] p-1 text-[var(--app-text-muted)] transition hover:text-[var(--app-text)]"
              aria-label="Toggle theme"
            >
              {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
            <span className="rounded-full border border-[var(--app-border)] px-2 py-0.5 text-[10px] uppercase tracking-[0.2em] text-[var(--app-text-muted)]">
              Secure
            </span>
          </div>
        </div>
        <p className="mb-6 text-sm text-[var(--app-text-muted)]">
          Access your terminal workspace securely from any device.
        </p>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <label htmlFor="password" className="mb-2 block text-sm font-medium text-[var(--app-text)]">
            Password
          </label>
          <input
            id="password"
            type="password"
            autoFocus
            value={password}
            onChange={(event) => onPasswordChange(event.target.value)}
            className="mb-4 block w-full rounded-lg border border-[var(--app-border)] bg-[var(--app-surface-muted)] px-4 py-2.5 text-sm text-[var(--app-text)] placeholder:text-[var(--app-text-muted)] outline-none focus:border-[var(--app-accent)] focus:ring-1 focus:ring-[var(--app-accent)]"
            placeholder="Enter password"
          />
          {loginError && <p className="mb-3 text-sm text-rose-500">{loginError}</p>}
          <Button type="submit" className="w-full" disabled={loggingIn || !password}>
            {loggingIn ? "Authenticating…" : "Login"}
          </Button>
        </form>
      </div>
    </div>
  );
}
