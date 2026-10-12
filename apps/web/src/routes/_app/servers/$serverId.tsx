import { Alert, Card, Spinner } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { containersQuery, serverQuery } from "../../../api/servers";
import type { Container, ContainerList, Server } from "../../../api/types";
import { ContainerActions } from "../../../components/ContainerActions";
import { ContainerLogs } from "../../../components/ContainerLogs";
import { ContainerStateChip } from "../../../components/ContainerStateChip";
import { QueryError } from "../../../components/QueryError";
import { ServerStatusChip } from "../../../components/ServerStatusChip";
import { timeAgo } from "../../../lib/time";
import { useHasRole } from "../../../lib/useHasRole";

export const Route = createFileRoute("/_app/servers/$serverId")({
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(serverQuery(params.serverId)),
      context.queryClient.prefetchQuery(containersQuery(params.serverId)),
    ]),
  component: ServerPage,
});

function ServerPage() {
  const { serverId } = Route.useParams();
  const server = useQuery(serverQuery(serverId));
  const containers = useQuery(containersQuery(serverId));

  if (server.isPending) {
    return <Spinner aria-label="Loading server" />;
  }
  if (server.isError) {
    return <QueryError error={server.error} onRetry={() => server.refetch()} />;
  }
  return (
    <div className="flex flex-col gap-6">
      <ServerSummary server={server.data} />
      <section aria-labelledby="containers-heading" className="flex flex-col gap-3">
        <h2 id="containers-heading" className="font-semibold">
          Containers
        </h2>
        {containers.isPending && <Spinner aria-label="Loading containers" />}
        {containers.isError && (
          <QueryError error={containers.error} onRetry={() => containers.refetch()} />
        )}
        {containers.isSuccess && (
          <Containers
            list={containers.data}
            serverId={serverId}
            projectId={server.data.project_id}
          />
        )}
      </section>
    </div>
  );
}

function ServerSummary({ server }: { server: Server }) {
  const facts: [string, string][] = [
    ["Project", server.project_name],
    ["Last heartbeat", timeAgo(server.last_seen_at)],
    ["Agent", server.agent_version ?? "unknown"],
    ["Docker", server.docker.available ? (server.docker.version ?? "available") : "not reachable"],
  ];
  if (server.resources) {
    facts.push(
      ["CPU", `${Math.round(server.resources.cpu_percent)}%`],
      ["Memory", `${Math.round(server.resources.memory_percent)}%`],
      ["Disk", `${Math.round(server.resources.disk_percent)}%`],
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/servers" className="text-sm text-[var(--muted)] hover:text-[var(--foreground)]">
          ← Servers
        </Link>
        <div className="mt-1 flex flex-wrap items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">{server.name}</h1>
          <ServerStatusChip status={server.status} />
        </div>
      </div>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm sm:grid-cols-4 lg:grid-cols-7">
        {facts.map(([label, value]) => (
          <div key={label}>
            <dt className="text-xs text-[var(--muted)]">{label}</dt>
            <dd className="truncate">{value}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

type ContainersProps = { list: ContainerList; serverId: string; projectId: string };

function Containers({ list, serverId, projectId }: ContainersProps) {
  const canOperate = useHasRole(projectId, "developer");
  return (
    <>
      {list.stale ? (
        <Alert status="warning">
          <Alert.Content>
            <Alert.Description>
              {list.observed_at
                ? `This list was reported ${timeAgo(list.observed_at)} and may be out of date: the Agent has not sent a fresh snapshot.`
                : "The Agent has not reported any containers yet."}
            </Alert.Description>
          </Alert.Content>
        </Alert>
      ) : (
        <p className="text-xs text-[var(--muted)]">
          Reported by the Agent {timeAgo(list.observed_at)}.
        </p>
      )}
      {list.containers.length === 0 ? (
        <Card className="px-6 py-10 text-center text-sm text-[var(--muted)]">
          No containers on this server.
        </Card>
      ) : (
        <Card className="p-0">
          <ul className="divide-y divide-[var(--separator)]">
            {list.containers.map((container) => (
              <ContainerRow
                key={container.container_id}
                container={container}
                actions={
                  <div className="flex items-center gap-2">
                    <ContainerLogs serverId={serverId} container={container} />
                    {canOperate && <ContainerActions serverId={serverId} container={container} />}
                  </div>
                }
              />
            ))}
          </ul>
        </Card>
      )}
    </>
  );
}

function ContainerRow({ container, actions }: { container: Container; actions: ReactNode }) {
  return (
    <li className="flex flex-wrap items-center gap-x-6 gap-y-1 px-4 py-3">
      <div className="min-w-48 flex-1">
        <p className="font-medium">{container.name}</p>
        <p className="truncate font-mono text-xs text-[var(--muted)]">{container.image}</p>
      </div>
      <ContainerStateChip state={container.state} />
      <p className="w-40 text-sm text-[var(--muted)]">{container.status_text}</p>
      {actions}
    </li>
  );
}
