import { Button, Card, ListBox, Select } from "@heroui/react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { listJobs } from "../../../api/jobs";
import type { JobStatus } from "../../../api/types";
import { JobStatusChip } from "../../../components/JobStatusChip";
import { PageHeader } from "../../../components/PageHeader";
import { QueryBoundary } from "../../../components/QueryBoundary";
import { describeJob, duration } from "../../../lib/describeJob";
import { timeAgo } from "../../../lib/time";

const statuses: (JobStatus | "all")[] = [
  "all",
  "queued",
  "running",
  "succeeded",
  "failed",
  "cancelled",
];

const jobTypes = [
  { id: "build", label: "Builds" },
  { id: "deploy_image", label: "Deployments" },
  { id: "container_action", label: "Container actions" },
  { id: "fetch_logs", label: "Log fetches" },
];

export const Route = createFileRoute("/_app/jobs/")({
  validateSearch: (search: Record<string, unknown>): { status?: JobStatus; type?: string } => ({
    status:
      statuses.includes(search.status as JobStatus) && search.status !== "all"
        ? (search.status as JobStatus)
        : undefined,
    type: jobTypes.some((t) => t.id === search.type) ? (search.type as string) : undefined,
  }),
  component: JobsPage,
});

function JobsPage() {
  const { status, type } = Route.useSearch();
  const navigate = Route.useNavigate();
  const jobs = useInfiniteQuery({
    queryKey: ["jobs", { status, type }],
    queryFn: ({ pageParam, signal }) => listJobs({ status, type, cursor: pageParam }, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    // Only this page polls, and only the first page while it is open.
    refetchInterval: 10_000,
  });

  return (
    <>
      <PageHeader
        title="Jobs"
        description="Builds and server operations, newest first."
        actions={
          <div className="flex flex-wrap gap-2">
            <Select
              aria-label="Filter by type"
              className="w-48"
              selectedKey={type ?? "all"}
              onSelectionChange={(key) =>
                navigate({
                  search: (prev) => ({ ...prev, type: key === "all" ? undefined : String(key) }),
                })
              }
            >
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {[{ id: "all", label: "All types" }, ...jobTypes].map((t) => (
                    <ListBox.Item key={t.id} id={t.id} textValue={t.label}>
                      {t.label}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
            <Select
              aria-label="Filter by status"
              className="w-40"
              selectedKey={status ?? "all"}
              onSelectionChange={(key) =>
                navigate({
                  search: (prev) => ({
                    ...prev,
                    status: key === "all" ? undefined : (key as JobStatus),
                  }),
                })
              }
            >
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {statuses.map((s) => (
                    <ListBox.Item key={s} id={s} textValue={s}>
                      {s === "all" ? "All statuses" : s}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          </div>
        }
      />
      <QueryBoundary
        query={jobs}
        label="jobs"
        isEmpty={(data) => data.pages[0]?.items.length === 0}
        empty={
          <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
            {status || type
              ? "No jobs match these filters."
              : "No jobs yet. They appear when you push code or operate a container."}
          </Card>
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {data.pages.flatMap((page) =>
                page.items.map((job) => (
                  <li
                    key={job.id}
                    className="flex flex-wrap items-center gap-x-6 gap-y-1 px-4 py-3"
                  >
                    <Link
                      to="/jobs/$jobId"
                      params={{ jobId: job.id }}
                      className="min-w-56 flex-1 truncate font-medium hover:underline"
                    >
                      {describeJob(job)}
                    </Link>
                    <JobStatusChip status={job.status} />
                    <p className="w-28 text-sm text-[var(--muted)]">{timeAgo(job.created_at)}</p>
                    <p className="w-16 text-end text-sm tabular-nums text-[var(--muted)]">
                      {duration(job.started_at, job.finished_at)}
                    </p>
                  </li>
                )),
              )}
            </ul>
          </Card>
        )}
      </QueryBoundary>
      {jobs.hasNextPage && (
        <div className="mt-4 flex justify-center">
          <Button
            variant="secondary"
            isPending={jobs.isFetchingNextPage}
            onPress={() => jobs.fetchNextPage()}
          >
            Load more
          </Button>
        </div>
      )}
    </>
  );
}
