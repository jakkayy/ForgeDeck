import { useQuery } from "@tanstack/react-query";
import { projectsQuery } from "../api/projects";
import type { Role } from "../api/types";

const rank: Record<Role, number> = { viewer: 1, developer: 2, admin: 3, owner: 4 };

/** Whether the signed-in user's role in projectId is at least minimum. */
export function useHasRole(projectId: string, minimum: Role): boolean {
  const projects = useQuery(projectsQuery);
  const role = projects.data?.items.find((p) => p.id === projectId)?.role;
  return role !== undefined && rank[role] >= rank[minimum];
}
