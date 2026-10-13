import { Alert } from "@heroui/react";
import { Link } from "@tanstack/react-router";
import type { Deployment } from "../../api/environments";
import { deploymentErrorHelp } from "../../lib/deploymentErrors";

const stageLabel = {
  pull: "pulling the image",
  start: "starting",
  health_check: "the health check",
};

/**
 * Why a deployment failed: what happened (the server's message says what state
 * the environment is in), likely causes, and the log to read next.
 */
export function DeploymentFailure({ deployment }: { deployment: Deployment }) {
  if (deployment.status !== "failed" && deployment.status !== "cancelled") {
    return null;
  }
  if (deployment.status === "cancelled") {
    return (
      <Alert status="warning">
        <Alert.Content>
          <Alert.Description>This deployment was cancelled before it finished.</Alert.Description>
        </Alert.Content>
      </Alert>
    );
  }

  const code = deployment.error?.code ?? "unknown_error";
  const { title, steps } = deploymentErrorHelp(code);
  return (
    <Alert status="danger">
      <Alert.Content>
        <Alert.Title>
          {title}
          {deployment.stage && ` (while ${stageLabel[deployment.stage]})`}
        </Alert.Title>
        <Alert.Description className="flex flex-col gap-2">
          {deployment.error?.message && <span>{deployment.error.message}</span>}
          <ul className="list-disc ps-5">
            {steps.map((step) => (
              <li key={step}>{step}</li>
            ))}
          </ul>
          <span className="text-xs">
            Error code <code className="font-mono">{code}</code>
            {deployment.job_id && (
              <>
                {" · "}
                <Link to="/jobs/$jobId" params={{ jobId: deployment.job_id }} className="underline">
                  Open the log
                </Link>
              </>
            )}
          </span>
        </Alert.Description>
      </Alert.Content>
    </Alert>
  );
}
