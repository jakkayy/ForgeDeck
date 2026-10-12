import { createRouter } from "@tanstack/react-router";
import { meQuery } from "./api/auth";
import { queryClient, setUnauthorizedHandler } from "./api/queryClient";
import { routeTree } from "./routeTree.gen";

export const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: "intent",
  scrollRestoration: true,
});

// A session can end while a page is open (logout elsewhere, expiry).
setUnauthorizedHandler(() => {
  queryClient.removeQueries({ queryKey: meQuery.queryKey });
  if (router.state.location.pathname !== "/login") {
    router.navigate({ to: "/login", search: { redirect: router.state.location.href } });
  }
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
