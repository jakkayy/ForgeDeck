import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { lazy, Suspense } from "react";

export type RouterContext = { queryClient: QueryClient };

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
