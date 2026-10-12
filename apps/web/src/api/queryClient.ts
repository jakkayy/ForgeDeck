import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./client";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      // Client errors (4xx) will not succeed on retry; network blips might.
      retry: (failureCount, error) =>
        !(error instanceof ApiError && error.status < 500) && failureCount < 2,
    },
  },
});
