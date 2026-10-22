import type { Deployment, DeploymentStage } from "../../api/environments";

type StepId = "queued" | DeploymentStage | "healthy";

const steps: { id: StepId; label: string }[] = [
  { id: "queued", label: "Queued" },
  { id: "pull", label: "Pulling image" },
  { id: "start", label: "Starting" },
  { id: "health_check", label: "Health check" },
  { id: "healthy", label: "Healthy" },
];

type StepState = "done" | "active" | "failed" | "pending";

/** Where a deployment is in its stages; a failure marks the stage it failed at. */
export function stepStates(d: Pick<Deployment, "status" | "stage">): StepState[] {
  const stageIndex = (stage: DeploymentStage | null) =>
    steps.findIndex((s) => s.id === (stage ?? "pull"));

  switch (d.status) {
    case "queued":
      return steps.map((_, i) => (i === 0 ? "active" : "pending"));
    case "running": {
      const current = stageIndex(d.stage);
      return steps.map((_, i) => (i < current ? "done" : i === current ? "active" : "pending"));
    }
    case "succeeded":
      return steps.map(() => "done");
    case "failed": {
      const failedAt = stageIndex(d.stage);
      return steps.map((_, i) => (i < failedAt ? "done" : i === failedAt ? "failed" : "pending"));
    }
    default:
      // cancelled: nothing is in progress, and nothing is claimed as done.
      return steps.map(() => "pending");
  }
}

const marker: Record<StepState, string> = {
  done: "bg-[var(--success)] text-[var(--success-foreground)]",
  active: "bg-[var(--accent)] text-[var(--accent-foreground)] animate-pulse",
  failed: "bg-[var(--danger)] text-[var(--danger-foreground)]",
  pending: "bg-[var(--default)] text-[var(--muted)]",
};

const symbol: Record<StepState, string> = { done: "✓", active: "●", failed: "✕", pending: "" };

export function DeploymentStages({ deployment }: { deployment: Deployment }) {
  const states = stepStates(deployment);
  return (
    <ol className="flex flex-wrap items-center gap-x-2 gap-y-1" aria-label="Deployment progress">
      {steps.map((step, i) => {
        const state = states[i] ?? "pending";
        return (
          <li
            key={step.id}
            aria-current={state === "active" ? "step" : undefined}
            className="flex items-center gap-2 text-xs"
          >
            <span
              className={`flex size-5 items-center justify-center rounded-full text-[10px] ${marker[state]}`}
              aria-hidden="true"
            >
              {symbol[state]}
            </span>
            <span className={state === "pending" ? "text-[var(--muted)]" : "font-medium"}>
              {step.label}
              <span className="sr-only"> ({state})</span>
            </span>
            {i < steps.length - 1 && (
              <span aria-hidden="true" className="h-px w-4 bg-[var(--separator)]" />
            )}
          </li>
        );
      })}
    </ol>
  );
}
