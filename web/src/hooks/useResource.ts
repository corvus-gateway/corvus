import { useCallback, useEffect, useState } from "react";
import { api, isMissing } from "../api";

export interface Resource<T> {
  /** The response for the current path; null until it arrived (also right after the path changed). */
  data: T | null;
  /** The last failure for the current path, also when older data is still shown (a failed refresh). */
  error: unknown;
  loading: boolean; // no response for the current path yet
  fetching: boolean; // any request in flight (first load, poll, reload)
  missing: boolean; // endpoint not implemented (404/405/501)
  reload: () => void;
}

interface Got<T> { path: string; data: T }
interface Failed { path: string; error: unknown }

/**
 * GET a JSON resource. Optional polling (ms) pauses while the tab is hidden and
 * refreshes immediately when it becomes visible again. Data and errors belong to the
 * path they were fetched for: after the path changes (time range, page) the old data is
 * not passed off as the new one. A failed refresh keeps the data and reports the error.
 */
export function useResource<T>(path: string | null, opts: { poll?: number } = {}): Resource<T> {
  const { poll = 0 } = opts;
  const [got, setGot] = useState<Got<T> | null>(null);
  const [failed, setFailed] = useState<Failed | null>(null);
  const [fetching, setFetching] = useState(false);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    if (path === null) return;
    let alive = true;
    let inflight = false;
    const run = async () => {
      if (inflight) return;
      inflight = true;
      setFetching(true);
      try {
        const d = await api.get<T>(path);
        if (alive) { setGot({ path, data: d }); setFailed(null); }
      } catch (e) {
        if (alive) setFailed({ path, error: e });
      } finally {
        inflight = false;
        if (alive) setFetching(false);
      }
    };
    run();
    let id = 0;
    if (poll > 0) id = window.setInterval(() => { if (!document.hidden) run(); }, poll);
    const onVis = () => { if (!document.hidden && poll > 0) run(); };
    document.addEventListener("visibilitychange", onVis);
    return () => { alive = false; setFetching(false); if (id) window.clearInterval(id); document.removeEventListener("visibilitychange", onVis); };
  }, [path, poll, tick]);

  const reload = useCallback(() => setTick((x) => x + 1), []);
  const has = path !== null && got?.path === path;
  const error = path !== null && failed?.path === path ? failed.error : null;
  return { data: has ? got!.data : null, error, loading: path !== null && !has && !error, fetching, missing: isMissing(error), reload };
}
