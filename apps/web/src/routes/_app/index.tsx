import { createFileRoute, redirect } from "@tanstack/react-router";

// The Overview page arrives in a later phase; servers are the main view until then.
export const Route = createFileRoute("/_app/")({
  beforeLoad: () => {
    throw redirect({ to: "/servers" });
  },
});
