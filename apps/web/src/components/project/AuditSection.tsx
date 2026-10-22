import { Button, Card, ListBox, Select } from "@heroui/react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { listAudit } from "../../api/audit";
import { auditActions, describeAudit } from "../../lib/describeAudit";
import { timeAgo } from "../../lib/time";
import { QueryBoundary } from "../QueryBoundary";

export function AuditSection({ projectId }: { projectId: string }) {
  const [action, setAction] = useState<string | undefined>();
  const events = useInfiniteQuery({
    queryKey: ["projects", projectId, "audit", { action }],
    queryFn: ({ pageParam, signal }) => listAudit(projectId, { action, cursor: pageParam }, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    // People open this to see what just happened, so never show a cached copy.
    staleTime: 0,
    refetchOnMount: "always",
  });
  const entries = events.data?.pages.flatMap((page) => page.items) ?? [];

  return (
    <section aria-label="Audit log" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="max-w-xl text-sm text-[var(--muted)]">
          Changes to this project, newest first. Secret values and credentials are never recorded.
        </p>
        <Select
          aria-label="Filter by action"
          className="w-56"
          selectedKey={action ?? "all"}
          onSelectionChange={(key) => setAction(key === "all" ? undefined : String(key))}
        >
          <Select.Trigger>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              {["all", ...auditActions].map((a) => (
                <ListBox.Item key={a} id={a} textValue={a}>
                  {a === "all" ? "All actions" : a}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
      </div>
      <QueryBoundary
        query={events}
        label="audit log"
        isEmpty={() => entries.length === 0}
        empty={
          <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
            {action ? "Nothing matches this filter." : "Nothing has been recorded yet."}
          </Card>
        }
      >
        {() => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {entries.map((entry) => (
                <li
                  key={entry.id}
                  className="flex flex-col gap-0.5 px-4 py-3 sm:flex-row sm:items-baseline sm:gap-4"
                >
                  <p className="min-w-0 flex-1 text-sm">
                    <span className="font-medium">{entry.actor}</span> {describeAudit(entry)}
                  </p>
                  <p className="font-mono text-xs text-[var(--muted)]">{entry.action}</p>
                  <time
                    dateTime={entry.created_at}
                    title={new Date(entry.created_at).toLocaleString()}
                    className="w-28 text-sm text-[var(--muted)] sm:text-end"
                  >
                    {timeAgo(entry.created_at)}
                  </time>
                </li>
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
      {events.hasNextPage && (
        <div className="flex justify-center">
          <Button
            variant="secondary"
            isPending={events.isFetchingNextPage}
            onPress={() => events.fetchNextPage()}
          >
            Load more
          </Button>
        </div>
      )}
    </section>
  );
}
