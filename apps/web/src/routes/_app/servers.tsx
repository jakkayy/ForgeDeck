import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_app/servers")({
  component: () => <h1 className="text-xl font-semibold">Servers</h1>,
});
