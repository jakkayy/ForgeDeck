import { Chip } from "@heroui/react";
import type { ServerStatus } from "../api/types";

const appearance: Record<
  ServerStatus,
  { label: string; color: "success" | "warning" | "danger" | "default" }
> = {
  online: { label: "Online", color: "success" },
  unreachable: { label: "Unreachable", color: "warning" },
  offline: { label: "Offline", color: "danger" },
  pending: { label: "Waiting for Agent", color: "default" },
};

export function ServerStatusChip({ status }: { status: ServerStatus }) {
  const { label, color } = appearance[status];
  return (
    <Chip size="sm" variant="soft" color={color}>
      <span aria-hidden="true" className="me-1">
        ●
      </span>
      {label}
    </Chip>
  );
}
