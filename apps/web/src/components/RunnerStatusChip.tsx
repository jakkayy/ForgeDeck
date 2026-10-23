import { Chip } from "@heroui/react";
import type { RunnerStatus } from "../api/runners";

const appearance: Record<RunnerStatus, { label: string; color: "success" | "danger" | "default" }> =
  {
    online: { label: "Online", color: "success" },
    offline: { label: "Offline", color: "danger" },
    pending: { label: "Waiting for Runner", color: "default" },
  };

export function RunnerStatusChip({ status }: { status: RunnerStatus }) {
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
