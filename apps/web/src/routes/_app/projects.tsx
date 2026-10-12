import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_app/projects")({
  component: () => <h1 className="text-xl font-semibold">Projects</h1>,
});
