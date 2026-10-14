import { Card } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import {
  type Overview,
  type OverviewDeployment,
  overviewQuery,
  recentJobsQuery,
} from "../../api/overview";
import { EmptyState } from "../../components/EmptyState";
import { JobStatusChip } from "../../components/JobStatusChip";
import { PageHeader } from "../../components/PageHeader";
import { QueryBoundary } from "../../components/QueryBoundary";
import { describeJob } from "../../lib/describeJob";
import { timeAgo } from "../../lib/time";

export const Route = createFileRoute("/_app/")({
  loader: ({ context }) => context.queryClient.prefetchQuery(overviewQuery),
  component: OverviewPage,
});

function OverviewPage() {
  const overview = useQuery(overviewQuery);

  return (
    <>
      <PageHeader title="Overview" description="What is running and what needs attention." />
      <QueryBoundary query={overview} label="overview">
        {(data) => <Summary overview={data} />}
      </QueryBoundary>
    </>
  );
}

function Summary({ overview }: { overview: Overview }) {
  const { servers, environments } = overview;
  if (servers.total === 0 && environments.total === 0) {
    return (
      <EmptyState
        title="Nothing here yet"
        description="Create a project, add a server, and deploy an application. This page then shows servers, deployments, and failures across all your projects."
        action={
          <Link to="/projects" className="text-sm font-medium underline">
            Go to projects
          </Link>
        }
      />
    );
  }
  const unhealthyServers = servers.unreachable + servers.offline;

  return (
    <div className="flex flex-col gap-6">
      <dl className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label="Servers online" value={`${servers.online} / ${servers.total}`} to="/servers" />
        <Stat
          label="Servers down"
          value={unhealthyServers}
          tone={unhealthyServers > 0 ? "danger" : undefined}
          to="/servers"
        />
        <Stat
          label="Environments running"
          value={`${environments.succeeded} / ${environments.total}`}
          hint="Last deployment succeeded"
        />
        <Stat
          label="Failed deployments"
          value={environments.failed}
          tone={environments.failed > 0 ? "danger" : undefined}
          hint={
            environments.in_progress > 0 ? `${environments.in_progress} in progress` : undefined
          }
        />
      </dl>

      {(overview.failing_deployments.length > 0 || unhealthyServers > 0) && (
        <Section title="Needs attention">
          <ul className="divide-y divide-[var(--separator)]">
            {unhealthyServers > 0 && (
              <li className="px-4 py-3 text-sm">
                <Link to="/servers" className="font-medium hover:underline">
                  {unhealthyServers === 1 ? "1 server is" : `${unhealthyServers} servers are`} not
                  reporting
                </Link>
                <p className="text-xs text-[var(--muted)]">
                  Check that the Agent is running and can reach ForgeDeck.
                </p>
              </li>
            )}
            {overview.failing_deployments.map((d) => (
              <DeploymentRow key={d.id} deployment={d} showError />
            ))}
          </ul>
        </Section>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <Section title="Recent deployments">
          {overview.recent_deployments.length === 0 ? (
            <p className="px-4 py-6 text-sm text-[var(--muted)]">Nothing has been deployed yet.</p>
          ) : (
            <ul className="divide-y divide-[var(--separator)]">
              {overview.recent_deployments.map((d) => (
                <DeploymentRow key={d.id} deployment={d} />
              ))}
            </ul>
          )}
        </Section>
        <RecentActivity />
      </div>
    </div>
  );
}

function Stat({
  label,
  value,
  hint,
  tone,
  to,
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  tone?: "danger";
  to?: "/servers";
}) {
  return (
    <Card className="relative h-full gap-1 p-4">
      <dt className="text-xs text-[var(--muted)]">
        {to ? (
          // Stretched over the whole card, so the card is the click target.
          <Link to={to} className="after:absolute after:inset-0 after:content-['']">
            {label}
          </Link>
        ) : (
          label
        )}
      </dt>
      <dd
        className={`text-2xl font-semibold tabular-nums ${tone === "danger" ? "text-[var(--danger)]" : ""}`}
      >
        {value}
      </dd>
      {hint && <dd className="text-xs text-[var(--muted)]">{hint}</dd>}
    </Card>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section aria-label={title}>
      <h2 className="mb-2 text-sm font-semibold">{title}</h2>
      <Card className="p-0">{children}</Card>
    </section>
  );
}

function DeploymentRow({
  deployment: d,
  showError,
}: {
  deployment: OverviewDeployment;
  showError?: boolean;
}) {
  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3">
      <div className="min-w-40 flex-1">
        <Link
          to="/environments/$environmentId"
          params={{ environmentId: d.environment_id }}
          className="text-sm font-medium hover:underline"
        >
          {d.application_name} · {d.environment_name}
        </Link>
        <p className="text-xs text-[var(--muted)]">
          {d.project_name} · {d.commit_sha.slice(0, 7)} · {timeAgo(d.created_at)}
          {showError && d.error_code ? ` · ${d.error_code}` : ""}
        </p>
      </div>
      <JobStatusChip status={d.status} />
    </li>
  );
}

function RecentActivity() {
  const jobs = useQuery(recentJobsQuery);
  return (
    <Section title="Recent jobs">
      <QueryBoundary
        query={jobs}
        label="jobs"
        isEmpty={(data) => data.items.length === 0}
        empty={<p className="px-4 py-6 text-sm text-[var(--muted)]">No jobs yet.</p>}
      >
        {(data) => (
          <ul className="divide-y divide-[var(--separator)]">
            {data.items.map((job) => (
              <li key={job.id} className="flex items-center gap-x-4 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <Link
                    to="/jobs/$jobId"
                    params={{ jobId: job.id }}
                    className="block truncate text-sm font-medium hover:underline"
                  >
                    {describeJob(job)}
                  </Link>
                  <p className="text-xs text-[var(--muted)]">{timeAgo(job.created_at)}</p>
                </div>
                <JobStatusChip status={job.status} />
              </li>
            ))}
          </ul>
        )}
      </QueryBoundary>
      <div className="border-t border-[var(--separator)] px-4 py-2 text-sm">
        <Link to="/jobs" className="text-[var(--muted)] hover:underline">
          All jobs
        </Link>
      </div>
    </Section>
  );
}
