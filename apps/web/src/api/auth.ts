import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { User } from "./types";

export const meQuery = queryOptions({
  queryKey: ["auth", "me"],
  queryFn: ({ signal }) => api<{ user: User }>("/auth/me", { signal }).then((r) => r.user),
  staleTime: 5 * 60_000,
  retry: false,
});

export function login(email: string, password: string) {
  return api<{ user: User }>("/auth/login", { method: "POST", body: { email, password } }).then(
    (r) => r.user,
  );
}

export function logout() {
  return api<void>("/auth/logout", { method: "POST" });
}
