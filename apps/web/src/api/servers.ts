import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { ContainerList, EnrollmentToken, List, Server } from "./types";

/** Matches the Agent heartbeat interval, so status is never more than one beat behind. */
export const SERVER_REFRESH_MS = 20_000;

export const serversQuery = queryOptions({
  queryKey: ["servers"],
  queryFn: ({ signal }) => api<List<Server>>("/servers?limit=100", { signal }),
  refetchInterval: SERVER_REFRESH_MS,
});

export function createEnrollmentToken(projectId: string, serverName: string) {
  return api<EnrollmentToken>(`/projects/${projectId}/servers/enrollment-tokens`, {
    method: "POST",
    body: { server_name: serverName },
  });
}

export const serverQuery = (serverId: string) =>
  queryOptions({
    queryKey: ["servers", serverId],
    queryFn: ({ signal }) => api<Server>(`/servers/${serverId}`, { signal }),
    refetchInterval: SERVER_REFRESH_MS,
  });

export const containersQuery = (serverId: string) =>
  queryOptions({
    queryKey: ["servers", serverId, "containers"],
    queryFn: ({ signal }) => api<ContainerList>(`/servers/${serverId}/containers`, { signal }),
    refetchInterval: SERVER_REFRESH_MS,
  });
