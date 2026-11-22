import { useCallback, useEffect, useState } from "react";
import { ApiError, api, type User } from "./api";
import { Button } from "./components/ui/button";
import { useI18n } from "./i18n";
import { Shell } from "./components/layout/Shell";
import { Logo } from "./components/Logo";
import { AppProvider } from "./lib/app";
import { LoginPage, SetupPage } from "./pages/Auth";

type Phase = { kind: "loading" } | { kind: "setup" } | { kind: "login" } | { kind: "down" } | { kind: "ready"; user: User };

export default function App() {
  const [phase, setPhase] = useState<Phase>({ kind: "loading" });

  const refresh = useCallback(async () => {
    try {
      const st = await api.get<{ setup_required: boolean; version: string }>("/api/status");
      if (st.setup_required) return setPhase({ kind: "setup" });
      const me = await api.get<User>("/api/auth/me");
      setPhase({ kind: "ready", user: me });
    } catch (e) {
      // No answer at all, or an error from a server that is starting or stopping, says
      // nothing about the session: wait for the server instead of showing a sign-in form
      // that cannot work.
      setPhase(e instanceof ApiError && e.status < 500 ? { kind: "login" } : { kind: "down" });
    }
  }, []);

  useEffect(() => { refresh(); }, [refresh]);
  useEffect(() => {
    const onUnauth = () => setPhase((p) => (p.kind === "ready" ? { kind: "login" } : p));
    window.addEventListener("corvus:unauthorized", onUnauth);
    return () => window.removeEventListener("corvus:unauthorized", onUnauth);
  }, []);

  // while the server cannot be reached: try again every few seconds
  const down = phase.kind === "down";
  useEffect(() => {
    if (!down) return;
    const id = window.setInterval(() => { if (!document.hidden) refresh(); }, 5000);
    return () => window.clearInterval(id);
  }, [down, refresh]);

  if (phase.kind === "down") return <ServerDown onRetry={refresh} />;
  if (phase.kind === "loading") {
    return <div className="grid h-dvh place-items-center bg-bg"><Logo className="h-10 w-auto animate-pulse text-fg" /></div>;
  }
  if (phase.kind === "setup") return <SetupPage onDone={(u) => setPhase({ kind: "ready", user: u })} />;
  if (phase.kind === "login") return <LoginPage onDone={() => refresh()} />;
  return (
    <AppProvider user={phase.user} setUser={(u) => setPhase({ kind: "ready", user: u })} logout={() => setPhase({ kind: "login" })}>
      <Shell />
    </AppProvider>
  );
}

function ServerDown({ onRetry }: { onRetry: () => void }) {
  const { t } = useI18n();
  return (
    <div role="alert" className="grid h-dvh place-items-center bg-bg px-4">
      <div className="max-w-md space-y-3 text-center">
        <Logo className="mx-auto h-10 w-auto text-fg" />
        <h1 className="text-lg font-semibold text-fg">{t("serverDownTitle")}</h1>
        <p className="text-sm text-fg-muted">{t("serverDownDesc")}</p>
        <Button onClick={onRetry}>{t("serverDownRetry")}</Button>
      </div>
    </div>
  );
}
