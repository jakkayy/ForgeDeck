import { QueryCache, QueryClient } from "@tanstack/react-query";
import { ApiError, isApiError } from "./client";

let onUnauthorized: () => void = () => {};

export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler;
}

export const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (error) => {
      if (isApiError(error, 401)) {
        onUnauthorized();
      }
    },
  }),
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      // Client errors (4xx) will not succeed on retry; network blips might.
      retry: (failureCount, error) =>
        !(error instanceof ApiError && error.status < 500) && failureCount < 2,
    },
  },
});
