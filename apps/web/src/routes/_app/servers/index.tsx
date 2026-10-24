import { Card, ListBox, Select } from "@heroui/react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { loadedItems } from "../../../api/paged";
import { serverListQuery } from "../../../api/servers";
import type { Server, ServerStatus } from "../../../api/types";
import { AddServerDialog } from "../../../components/AddServerDialog";
import { EmptyState } from "../../../components/EmptyState";
import { LoadMore } from "../../../components/LoadMore";
import { PageHeader } from "../../../components/PageHeader";
import { QueryBoundary } from "../../../components/QueryBoundary";
import { SearchBox } from "../../../components/SearchBox";
import { ServerStatusChip } from "../../../components/ServerStatusChip";
import { timeAgo } from "../../../lib/time";
import { useDebouncedValue } from "../../../lib/useDebouncedValue";

export const Route = createFileRoute("/_app/servers/")({
  loader: ({ context }) => context.queryClient.prefetchInfiniteQuery(serverListQuery({})),
  component: ServersPage,
});

const statuses: (ServerStatus | "all")[] = ["all", "online", "unreachable", "offline", "pending"];

function ServersPage() {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<ServerStatus | undefined>();
  const q = useDebouncedValue(search.trim()) || undefined;
  const servers = useInfiniteQuery(serverListQuery({ q, status }));
  const filtered = Boolean(q || status);
  const noServersAtAll =
    !filtered && servers.data !== undefined && loadedItems(servers.data).length === 0;

  return (
    <>
      <PageHeader
        title="Servers"
        description="Docker hosts running the ForgeDeck Agent."
        actions={noServersAtAll ? undefined : <AddServerDialog />}
      />
      {!noServersAtAll && (
        <div className="mb-4 flex flex-wrap items-center gap-3">
          <SearchBox
            label="Search servers"
            placeholder="Search by name"
            value={search}
            onChange={setSearch}
          />
          <Select
            aria-label="Filter by status"
            className="w-44"
            selectedKey={status ?? "all"}
            onSelectionChange={(key) =>
              setStatus(key === "all" ? undefined : (key as ServerStatus))
            }
          >
            <Select.Trigger>
              <Select.Value />
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover>
              <ListBox>
                {statuses.map((value) => (
                  <ListBox.Item key={value} id={value} textValue={value}>
                    {value === "all" ? "All statuses" : value}
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                ))}
              </ListBox>
            </Select.Popover>
          </Select>
        </div>
      )}
      <QueryBoundary
        query={servers}
        label="servers"
        isEmpty={(data) => loadedItems(data).length === 0}
        empty={
          filtered ? (
            <EmptyState title="No servers match" description="Try a different name or status." />
          ) : (
            <EmptyState
              title="No servers yet"
              description="Register your first Docker server: install the Agent and it connects to ForgeDeck over HTTPS, with no inbound ports."
              action={<AddServerDialog />}
            />
          )
        }
      >
        {(data) => (
          <Card className="p-0">
            <ul className="divide-y divide-[var(--separator)]">
              {loadedItems(data).map((server) => (
                <ServerRow key={server.id} server={server} />
              ))}
            </ul>
          </Card>
        )}
      </QueryBoundary>
      <LoadMore query={servers} />
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
