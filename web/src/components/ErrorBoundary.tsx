import { Component, type ErrorInfo, type ReactNode } from "react";
import { useI18n } from "../i18n";
import { Button } from "./ui/button";

interface Props { children: ReactNode; title: string; desc: string; updated: string; reload: string }
interface State { error: Error | null }

/** A lazily loaded page whose file is gone: the console was upgraded while this tab was open. */
const staleChunk = (e: Error) =>
  /dynamically imported module|module script failed|error loading dynamically|Failed to fetch|ChunkLoadError/i.test(`${e.name} ${e.message}`);

class Boundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: unknown): State {
    return { error: error instanceof Error ? error : new Error(String(error)) };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("page failed to render", error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    const stale = staleChunk(error);
    return (
      <div role="alert" className="mx-auto max-w-xl space-y-3 pt-16 text-center">
        <h1 className="text-lg font-semibold text-fg">{this.props.title}</h1>
        <p className="text-sm text-fg-muted">{stale ? this.props.updated : this.props.desc}</p>
        {!stale && <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-[3px] border border-line bg-surface p-3 text-left text-xs text-fg-muted">{error.message}</pre>}
        <Button variant="primary" onClick={() => window.location.reload()}>{this.props.reload}</Button>
      </div>
    );
  }
}

/**
 * Keeps a page that throws while rendering (or whose code cannot be loaded) from taking
 * the whole console down: the navigation stays, the page area shows what happened.
 * Give it a `key` that changes with the route so that moving on clears the error.
 */
export function ErrorBoundary({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  return (
    <Boundary title={t("crashTitle")} desc={t("crashDesc")} updated={t("crashUpdated")} reload={t("crashReload")}>
      {children}
    </Boundary>
  );
}
