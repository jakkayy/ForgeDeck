import { Alert, Button, Spinner } from "@heroui/react";
import type { ReactNode } from "react";
import { timeAgo } from "../lib/time";
import { QueryError } from "./QueryError";

type QueryLike<T> = {
  data: T | undefined;
  error: Error | null;
  isPending: boolean;
  isError: boolean;
  fetchStatus: "fetching" | "paused" | "idle";
  dataUpdatedAt: number;
  refetch: () => unknown;
};

type Props<T> = {
  query: QueryLike<T>;
  label: string;
  isEmpty?: (data: T) => boolean;
  empty?: ReactNode;
  children: (data: T) => ReactNode;
};

export function waitingFor<T>(query: QueryLike<T>, label: string): ReactNode {
  if (query.data !== undefined) {
    return null;
  }
  if (query.isError && query.error) {
    return <QueryError error={query.error} onRetry={() => query.refetch()} />;
  }
  return <Spinner aria-label={`Loading ${label}`} />;
}

/** A notice for stale data on screen; a network blip must not replace a page with an error. */
export function StaleNotice<T>({ query, label }: { query: QueryLike<T>; label: string }) {
  const offline = query.fetchStatus === "paused";
  if (query.data === undefined || !(query.isError || offline)) {
    return null;
  }
  return (
    <Alert status="warning" className="mb-3">
      <Alert.Content>
        <Alert.Title>{offline ? "You are offline" : `Could not refresh ${label}`}</Alert.Title>
        <Alert.Description>
          Showing what was loaded {timeAgo(new Date(query.dataUpdatedAt).toISOString())}.
          {query.error ? ` ${query.error.message}` : ""}
        </Alert.Description>
      </Alert.Content>
      {!offline && (
        <Button size="sm" variant="secondary" onPress={() => query.refetch()}>
          Try again
        </Button>
      )}
    </Alert>
  );
}

export function QueryBoundary<T>({ query, label, isEmpty, empty, children }: Props<T>) {
  const waiting = waitingFor(query, label);
  if (waiting || query.data === undefined) {
    return waiting;
  }
  return (
    <>
      <StaleNotice query={query} label={label} />
      {isEmpty?.(query.data) ? empty : children(query.data)}
    </>
  );
}
