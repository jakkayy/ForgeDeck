import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { lazy, Suspense } from "react";

export type RouterContext = { queryClient: QueryClient };

// Toasts are only raised by actions on loaded pages, and the queue is global,
// so the provider can arrive after first paint without losing messages.
const Toasts = lazy(() => import("../components/Toasts"));

export const Route = createRootRouteWithContext<RouterContext>()({
  component: Root,
});

function Root() {
  return (
    <>
      <Outlet />
      <Suspense>
        <Toasts />
      </Suspense>
    </>
  );
}
