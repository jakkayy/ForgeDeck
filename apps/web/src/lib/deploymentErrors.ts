export type DeploymentErrorHelp = {
  title: string;
  steps: string[];
};

const generic: DeploymentErrorHelp = {
  title: "The deployment failed",
  steps: ["Open the log for the details.", "Deploy again once the cause is fixed."],
};

const help: Record<string, DeploymentErrorHelp> = {
  registry_login_failed: {
    title: "The server could not log in to the registry",
    steps: [
      "Check the username and password of the registry in the project's Registries tab.",
      "Make sure the token has permission to pull images.",
    ],
  },
  pull_failed: {
    title: "The image could not be pulled",
    steps: [
      "Check that the server can reach the registry.",
      "Check that the image still exists in the registry: it may have been deleted or cleaned up.",
      "The service that was running was not touched.",
    ],
  },
  start_failed: {
    title: "The service could not be started",
    steps: [
      "Open the log: the most common cause is a host port that something else already uses.",
      "Check the ports and volumes in the environment's settings.",
    ],
  },
  health_check_failed: {
    title: "The new version did not become healthy",
    steps: [
      "Open the log: it ends with the last output of the failed service.",
      "Check the container port and the health check in the environment's settings.",
      "Fix the problem in the code, build again, and deploy the new build.",
    ],
  },
  compose_unavailable: {
    title: "Docker Compose is not installed on the server",
    steps: ["Install the Docker Compose plugin on the server, then deploy again."],
  },
  registry_credentials_unavailable: {
    title: "ForgeDeck could not read the registry credentials",
    steps: [
      "Re-enter the registry's password in the project's Registries tab.",
      "If the encryption keys of this ForgeDeck changed, stored credentials must be entered again.",
    ],
  },
  environment_not_found: {
    title: "The environment was deleted",
    steps: ["Nothing was deployed. Create the environment again to deploy."],
  },
  invalid_payload: {
    title: "The server refused the deployment",
    steps: [
      "The environment's settings did not pass the server's checks. Review them and deploy again.",
    ],
  },
  unsupported_job_type: {
    title: "This server's Agent is too old to deploy",
    steps: ["Upgrade the Agent on the server, then deploy again."],
  },
  lease_expired: {
    title: "The server stopped responding during the deployment",
    steps: [
      "Check that the server is online and that its Agent is running.",
      "The service may be in either state: check the server's containers, then deploy again.",
    ],
  },
  port_in_use: {
    title: "Another environment uses this port",
    steps: ["Change the host port of one of the environments."],
  },
};

export function deploymentErrorHelp(code: string): DeploymentErrorHelp {
  return help[code] ?? generic;
}
