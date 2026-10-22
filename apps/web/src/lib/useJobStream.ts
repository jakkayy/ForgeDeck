import { useEffect, useState } from "react";
import type { Job, LogLine } from "../api/types";

type Stream = { job: Job | null; lines: LogLine[]; ended: boolean };

const FLUSH_MS = 100;
/** A tab hidden this long stops following the stream and resumes when shown again. */
const HIDDEN_GRACE_MS = 60_000;

/** Follows a job's status and output over SSE; EventSource resumes via Last-Event-ID. */
export function useJobStream(jobId: string): Stream {
  const [state, setState] = useState<Stream>({ job: null, lines: [], ended: false });

  useEffect(() => {
    setState({ job: null, lines: [], ended: false });
    let source: EventSource | undefined;
    let lastLineId = "";
    let finished = false;
    let pending: LogLine[] = [];
    let flushTimer: ReturnType<typeof setTimeout> | undefined;
    let hiddenTimer: ReturnType<typeof setTimeout> | undefined;

    const flush = () => {
      flushTimer = undefined;
      if (pending.length === 0) {
        return;
      }
      const batch = pending;
      pending = [];
      setState((s) => ({ ...s, lines: s.lines.concat(batch) }));
    };

    const close = () => {
      source?.close();
      source = undefined;
    };

    const open = () => {
      const resume = lastLineId ? `?after=${lastLineId}` : "";
      const stream = new EventSource(`/api/v1/jobs/${jobId}/events${resume}`);
      source = stream;
      stream.addEventListener("status", (event) => {
        const job = JSON.parse((event as MessageEvent<string>).data) as Job;
        setState((s) => ({ ...s, job }));
      });
      stream.addEventListener("log", (event) => {
        const line = JSON.parse((event as MessageEvent<string>).data) as LogLine;
        lastLineId = `${line.attempt}.${line.sequence}`;
        pending.push(line);
        flushTimer ??= setTimeout(flush, FLUSH_MS);
      });
      stream.addEventListener("end", () => {
        finished = true;
        close();
        flush();
        setState((s) => ({ ...s, ended: true }));
      });
    };

    const onVisibility = () => {
      if (document.hidden) {
        hiddenTimer = setTimeout(close, HIDDEN_GRACE_MS);
        return;
      }
      clearTimeout(hiddenTimer);
      if (!source && !finished) {
        open();
      }
    };

    open();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      close();
      clearTimeout(flushTimer);
      clearTimeout(hiddenTimer);
    };
  }, [jobId]);

  return state;
}
