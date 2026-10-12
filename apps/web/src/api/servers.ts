import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { List, Server } from "./types";

/** Matches the Agent heartbeat interval, so status is never more than one beat behind. */
export const SERVER_REFRESH_MS = 20_000;

export const serversQuery = queryOptions({
  queryKey: ["servers"],
  queryFn: ({ signal }) => api<List<Server>>("/servers?limit=100", { signal }),
  refetchInterval: SERVER_REFRESH_MS,
});
