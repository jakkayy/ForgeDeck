import { Card } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { serversQuery } from "../../../api/servers";
import type { Server } from "../../../api/types";
import { AddServerDialog } from "../../../components/AddServerDialog";
import { EmptyState } from "../../../components/EmptyState";
import { PageHeader } from "../../../components/PageHeader";
import { QueryBoundary } from "../../../components/QueryBoundary";
import { ServerStatusChip } from "../../../components/ServerStatusChip";
import { timeAgo } from "../../../lib/time";

export const Route = createFileRoute("/_app/servers/")({
  loader: ({ context }) => context.queryClient.prefetchQuery(serversQuery),
  component: ServersPage,
});

function ServersPage() {
  const servers = useQuery(serversQuery);

  return (
    <>
      <PageHeader
        title="Servers"
        description="Docker hosts running the ForgeDeck Agent."
        actions={<AddServerDialog />}
      />
      <QueryBoundary
        query={servers}
        label="servers"
        isEmpty={(data) => data.items.length === 0}
        empty={
          <EmptyState
            title="No servers yet"
            description="Register your first Docker server: install the Agent and it connects to ForgeDeck over HTTPS, with no inbound ports."
          />
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {data.items.map((server) => (
                <ServerRow key={server.id} server={server} />
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
    </>
  );
}

function ServerRow({ server }: { server: Server }) {
  return (
    <li className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
      <div className="min-w-48 flex-1">
        <Link
          to="/servers/$serverId"
          params={{ serverId: server.id }}
          className="font-medium hover:underline"
        >
          {server.name}
        </Link>
        <p className="text-xs text-[var(--muted)]">{server.project_name}</p>
      </div>
      <ServerStatusChip status={server.status} />
      <p className="w-36 text-sm text-[var(--muted)]">Seen {timeAgo(server.last_seen_at)}</p>
      <Usage server={server} />
    </li>
  );
}

function Usage({ server }: { server: Server }) {
  if (!server.resources) {
    return <p className="w-56 text-sm text-[var(--muted)]">No usage reported yet</p>;
  }
  const { cpu_percent, memory_percent, disk_percent } = server.resources;
  return (
    <dl className="grid w-56 grid-cols-3 text-sm">
      {[
        ["CPU", cpu_percent],
        ["Memory", memory_percent],
        ["Disk", disk_percent],
      ].map(([label, value]) => (
        <div key={label}>
          <dt className="text-xs text-[var(--muted)]">{label}</dt>
          <dd className="tabular-nums">{Math.round(Number(value))}%</dd>
        </div>
      ))}
    </dl>
  );
}
