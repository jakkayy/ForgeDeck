import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

/**
 * Refreshes an environment's releases when its deployments stop being active.
 * A release is recorded in the same moment a deployment succeeds, so without
 * this the "running now" card and the release list would keep showing what was
 * true before the deployment, while its own row already says "succeeded".
 */
export function useRefreshReleases(environmentId: string, deploymentActive: boolean) {
  const queryClient = useQueryClient();
  const wasActive = useRef(false);
  useEffect(() => {
    if (wasActive.current && !deploymentActive) {
      queryClient.invalidateQueries({ queryKey: ["environments", environmentId, "releases"] });
    }
    wasActive.current = deploymentActive;
  }, [deploymentActive, environmentId, queryClient]);
}
