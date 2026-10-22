import { useQuery } from "@tanstack/react-query";
import { projectQuery } from "../api/projects";
import type { Role } from "../api/types";

const rank: Record<Role, number> = { viewer: 1, developer: 2, admin: 3, owner: 4 };

export function useHasRole(projectId: string, minimum: Role): boolean {
  const project = useQuery(projectQuery(projectId));
  const role = project.data?.role;
  return role !== undefined && rank[role] >= rank[minimum];
}
