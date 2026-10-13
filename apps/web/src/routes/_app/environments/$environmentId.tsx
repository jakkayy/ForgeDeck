import { Card, Spinner } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { applicationQuery } from "../../../api/applications";
import {
  type Deployment,
  deploymentsQuery,
  environmentQuery,
  releasesQuery,
} from "../../../api/environments";
import { serversQuery } from "../../../api/servers";
import { DeploymentStages } from "../../../components/environment/DeploymentStages";
import { JobStatusChip } from "../../../components/JobStatusChip";
import { QueryError } from "../../../components/QueryError";
import { ServerStatusChip } from "../../../components/ServerStatusChip";
import { duration } from "../../../lib/describeJob";
import { shortImage } from "../../../lib/serviceConfig";
import { timeAgo } from "../../../lib/time";

export const Route = createFileRoute("/_app/environments/$environmentId")({
  loader: ({ context, params }) =>
    context.queryClient.prefetchQuery(environmentQuery(params.environmentId)),
  component: EnvironmentPage,
});

function EnvironmentPage() {
  const { environmentId } = Route.useParams();
  const environment = useQuery(environmentQuery(environmentId));

  if (environment.isPending) {
    return <Spinner aria-label="Loading environment" />;
  }
  if (environment.isError) {
    return <QueryError error={environment.error} onRetry={() => environment.refetch()} />;
  }
  return (
    <div className="flex flex-col gap-6">
      <EnvironmentHeader environmentId={environmentId} />
      <CurrentRelease environmentId={environmentId} />
      <Deployments environmentId={environmentId} />
    </div>
  );
}

function EnvironmentHeader({ environmentId }: { environmentId: string }) {
  const environment = useQuery(environmentQuery(environmentId));
  const app = useQuery({
    ...applicationQuery(environment.data?.application_id ?? ""),
    enabled: !!environment.data,
  });
  const servers = useQuery(serversQuery);
  const server = servers.data?.items.find((s) => s.id === environment.data?.server_id);
  if (!environment.data) {
    return null;
  }

  return (
    <div>
      <Link
        to="/applications/$applicationId"
        params={{ applicationId: environment.data.application_id }}
        className="text-sm text-[var(--muted)] hover:text-[var(--foreground)]"
      >
        ← {app.data?.name ?? "Application"}
      </Link>
      <div className="mt-1 flex flex-wrap items-center gap-3">
        <h1 className="text-xl font-semibold tracking-tight">{environment.data.name}</h1>
        {server && <ServerStatusChip status={server.status} />}
      </div>
      <p className="mt-1 text-sm text-[var(--muted)]">
        {server ? `Runs on ${server.name}` : "Runs on a server of this project"}
        {environment.data.auto_deploy
          ? " · deploys every successful build"
          : " · deploys only when someone deploys"}
      </p>
    </div>
  );
}

function CurrentRelease({ environmentId }: { environmentId: string }) {
  const releases = useQuery(releasesQuery(environmentId, 1));
  const current = releases.data?.items[0];
  if (releases.isPending) {
    return <Spinner aria-label="Loading release" />;
  }
  return (
    <Card className="flex flex-col gap-1 px-4 py-3">
      <p className="text-xs text-[var(--muted)]">Running now</p>
      {current ? (
        <>
          <p className="font-mono text-sm">
            <code>{current.commit_sha.slice(0, 7)}</code> · {current.image_ref}
          </p>
          <p className="text-xs text-[var(--muted)]">Healthy since {timeAgo(current.created_at)}</p>
        </>
      ) : (
        <p className="text-sm text-[var(--muted)]">
          Nothing has been deployed yet. Deploy a build from the application page.
        </p>
      )}
    </Card>
  );
}

function Deployments({ environmentId }: { environmentId: string }) {
  const deployments = useQuery(deploymentsQuery(environmentId));
  return (
    <section aria-label="Deployments" className="flex flex-col gap-3">
      <h2 className="text-base font-semibold">Deployments</h2>
      {deployments.isPending && <Spinner aria-label="Loading deployments" />}
      {deployments.isError && (
        <QueryError error={deployments.error} onRetry={() => deployments.refetch()} />
      )}
      {deployments.isSuccess && deployments.data.items.length === 0 && (
        <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
          No deployments yet.
        </Card>
      )}
      {deployments.isSuccess && deployments.data.items.length > 0 && (
        <Card className="p-0">
          <ul className="divide-y divide-[var(--separator)]">
            {deployments.data.items.map((d) => (
              <DeploymentRow key={d.id} deployment={d} />
            ))}
          </ul>
        </Card>
      )}
    </section>
  );
}

function DeploymentRow({ deployment }: { deployment: Deployment }) {
  return (
    <li className="flex flex-col gap-2 px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <p className="min-w-48 flex-1 font-mono text-sm">
          <code>{deployment.commit_sha.slice(0, 7)}</code> · {shortImage(deployment.image_ref)}
          {deployment.source_release_id && (
            <span className="ms-2 font-sans text-xs text-[var(--muted)]">rollback</span>
          )}
        </p>
        <JobStatusChip status={deployment.status} />
        <p className="w-28 text-sm text-[var(--muted)]">{timeAgo(deployment.created_at)}</p>
        <p className="w-16 text-end text-sm tabular-nums text-[var(--muted)]">
          {duration(deployment.started_at, deployment.finished_at)}
        </p>
        {deployment.job_id && (
          <Link
            to="/jobs/$jobId"
            params={{ jobId: deployment.job_id }}
            className="text-sm text-[var(--accent)] hover:underline"
          >
            Log
          </Link>
        )}
      </div>
      <DeploymentStages deployment={deployment} />
      {deployment.error && (
        <p className="text-sm text-[var(--danger)]">{deployment.error.message}</p>
      )}
    </li>
  );
}
