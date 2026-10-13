import { Chip } from "@heroui/react";
import type { JobStatus } from "../api/types";

const colors: Record<JobStatus, "success" | "warning" | "danger" | "default" | "accent"> = {
  queued: "default",
  running: "accent",
  succeeded: "success",
  failed: "danger",
  cancelled: "warning",
};

export function JobStatusChip({ status }: { status: JobStatus }) {
  return (
    <Chip size="sm" variant="soft" color={colors[status]}>
      {status}
    </Chip>
  );
}
