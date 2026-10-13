import { useLayoutEffect, useRef } from "react";
import type { LogLine } from "../api/types";

// stderr is marked in the gutter rather than colored: many programs log
// ordinary output there, and an all-red log reads as an outage.
const streamStyle: Record<LogLine["stream"], string> = {
  stdout: "border-transparent",
  stderr: "border-[var(--warning)]",
  system: "border-transparent text-[var(--muted)]",
};

/** Splits Docker's "2026-10-12T20:08:09.43Z message" prefix into a short time. */
function splitTimestamp(message: string): { time: string | null; text: string } {
  const match = /^(\d{4}-\d{2}-\d{2}T(\d{2}:\d{2}:\d{2})[.\d]*Z) (.*)$/s.exec(message);
  return match ? { time: match[2] ?? null, text: match[3] ?? "" } : { time: null, text: message };
}

/**
 * Monospace log output. It opens at the newest line and keeps following new
 * lines while the reader is at the bottom; scrolling up pauses following.
 * Lines are rendered as text, never HTML.
 */
export function LogView({ lines }: { lines: LogLine[] }) {
  const ref = useRef<HTMLPreElement>(null);
  const atBottom = useRef(true);

  // biome-ignore lint/correctness/useExhaustiveDependencies: follow whenever lines are added
  useLayoutEffect(() => {
    if (atBottom.current) {
      ref.current?.scrollTo({ top: ref.current.scrollHeight });
    }
  }, [lines.length]);

  return (
    <pre
      ref={ref}
      onScroll={(event) => {
        const el = event.currentTarget;
        atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
      }}
      className="min-h-0 flex-1 overflow-auto rounded-lg bg-[var(--surface-secondary)] py-3 font-mono text-xs leading-5"
    >
      {lines.map((line) => {
        const { time, text } = splitTimestamp(line.message);
        return (
          <div
            key={`${line.attempt}.${line.sequence}`}
            className={`flex gap-3 border-s-2 px-3 ${streamStyle[line.stream]}`}
          >
            {time && <span className="shrink-0 select-none text-[var(--muted)]">{time}</span>}
            <span className="whitespace-pre-wrap break-all">{text}</span>
          </div>
        );
      })}
    </pre>
  );
}
