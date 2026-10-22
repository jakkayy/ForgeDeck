import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

/** Refreshes an environment's releases when its deployments stop being active. */
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
