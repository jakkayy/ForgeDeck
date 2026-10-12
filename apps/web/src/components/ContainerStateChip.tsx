import { Chip } from "@heroui/react";
import type { ContainerState } from "../api/types";

const colors: Record<ContainerState, "success" | "warning" | "danger" | "default"> = {
  running: "success",
  restarting: "warning",
  paused: "warning",
  created: "default",
  removing: "default",
  exited: "default",
  dead: "danger",
};

export function ContainerStateChip({ state }: { state: ContainerState }) {
  return (
    <Chip size="sm" variant="soft" color={colors[state]}>
      {state}
    </Chip>
  );
}
