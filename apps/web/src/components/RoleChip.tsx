import { Chip } from "@heroui/react";
import type { Role } from "../api/types";

export function RoleChip({ role }: { role: Role }) {
  return (
    <Chip
      size="sm"
      variant="soft"
      color={role === "owner" || role === "admin" ? "accent" : "default"}
    >
      {role}
    </Chip>
  );
}
