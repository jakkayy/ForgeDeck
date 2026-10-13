import { Button } from "@heroui/react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useLayoutEffect, useRef, useState } from "react";
import type { LogLine } from "../api/types";

// stderr is marked in the gutter rather than colored: many programs log
// ordinary output there, and an all-red log reads as an outage.
const streamStyle: Record<LogLine["stream"], string> = {
  stdout: "border-transparent",
  stderr: "border-[var(--warning)]",
  system: "border-transparent text-[var(--muted)]",
};

const ROW_HEIGHT_ESTIMATE = 20;
/** How close to the end still counts as "at the bottom", in pixels. */
const FOLLOW_SLACK = 24;

/** Splits Docker's "2026-10-12T20:08:09.43Z message" prefix into a short time. */
function splitTimestamp(message: string): { time: string | null; text: string } {
  const match = /^(\d{4}-\d{2}-\d{2}T(\d{2}:\d{2}:\d{2})[.\d]*Z) (.*)$/s.exec(message);
  return match ? { time: match[2] ?? null, text: match[3] ?? "" } : { time: null, text: message };
}

/**
 * Monospace log output. Only the lines in view are in the DOM, so a log of
 * hundreds of thousands of lines scrolls as smoothly as a short one. It opens
 * at the newest line and follows new lines while the reader is at the bottom;
 * scrolling up pauses following until "Jump to latest". Lines are rendered as
 * text, never HTML.
 *
 * Browser text selection only spans the rows that are rendered, so "Copy all"
 * copies the whole log.
 */
export function LogView({ lines }: { lines: LogLine[] }) {
  const scroller = useRef<HTMLDivElement>(null);
  const [following, setFollowing] = useState(true);
  const [copied, setCopied] = useState(false);
  const virtualizer = useVirtualizer({
    count: lines.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => ROW_HEIGHT_ESTIMATE,
    overscan: 30,
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: follow whenever lines are added
  useLayoutEffect(() => {
    if (following && lines.length > 0) {
      virtualizer.scrollToIndex(lines.length - 1, { align: "end" });
    }
  }, [lines.length, following]);

  const copyAll = async () => {
    try {
      await navigator.clipboard.writeText(lines.map((line) => line.message).join("\n"));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard access can be refused (permissions, insecure origin); nothing to recover.
    }
  };

  return (
    <div className="relative flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex items-center justify-between gap-3 text-xs text-[var(--muted)]">
        <span aria-live="off">{lines.length.toLocaleString()} lines</span>
        <Button size="sm" variant="ghost" onPress={copyAll}>
          {copied ? "Copied" : "Copy all"}
        </Button>
      </div>
      <div
        ref={scroller}
        role="log"
        aria-label="Log output"
        onScroll={(event) => {
          const el = event.currentTarget;
          setFollowing(el.scrollHeight - el.scrollTop - el.clientHeight < FOLLOW_SLACK);
        }}
        className="min-h-0 flex-1 overflow-auto rounded-lg bg-[var(--surface-secondary)] py-3 font-mono text-xs leading-5"
      >
        <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
          {virtualizer.getVirtualItems().map((item) => {
            const line = lines[item.index];
            if (!line) {
              return null;
            }
            const { time, text } = splitTimestamp(line.message);
            return (
              <div
                key={`${line.attempt}.${line.sequence}`}
                ref={virtualizer.measureElement}
                data-index={item.index}
                className={`absolute inset-x-0 top-0 flex gap-3 border-s-2 px-3 ${streamStyle[line.stream]}`}
                style={{ transform: `translateY(${item.start}px)` }}
              >
                {time && <span className="shrink-0 select-none text-[var(--muted)]">{time}</span>}
                <span className="whitespace-pre-wrap break-all">{text}</span>
              </div>
            );
          })}
        </div>
      </div>
      {!following && (
        <Button
          size="sm"
          variant="secondary"
          className="absolute end-4 bottom-4 shadow"
          onPress={() => setFollowing(true)}
        >
          Jump to latest
        </Button>
      )}
    </div>
  );
}
