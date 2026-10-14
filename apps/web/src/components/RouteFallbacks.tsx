import { type ErrorComponentProps, Link } from "@tanstack/react-router";
import type { ReactNode } from "react";

// These are registered on the router, so they load with the first page. They
// use plain elements on purpose: importing HeroUI here would pull it into the
// initial bundle.

function Centered({ children }: { children: ReactNode }) {
  return (
    <div className="mx-auto flex min-h-dvh max-w-lg flex-col justify-center gap-4 p-4">
      {children}
    </div>
  );
}

/** Shown for a URL that matches no page. */
export function NotFoundPage() {
  return (
    <Centered>
      <h1 className="text-xl font-semibold tracking-tight">Page not found</h1>
      <p className="text-sm text-[var(--muted)]">
        This address does not match a page. It may have been removed, or you may not have access to
        it.
      </p>
      <Link to="/" className="text-sm font-medium underline">
        Go to the overview
      </Link>
    </Centered>
  );
}

/** Shown when a page fails while rendering, instead of a blank screen. */
export function RouteErrorPage({ error, reset }: ErrorComponentProps) {
  return (
    <Centered>
      <div role="alert" className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold tracking-tight">Something went wrong on this page</h1>
        <p className="text-sm text-[var(--muted)]">
          {error instanceof Error ? error.message : "Unexpected error."}
        </p>
      </div>
      <div className="flex items-center gap-4">
        <button
          type="button"
          onClick={reset}
          className="rounded-lg bg-[var(--surface-secondary)] px-4 py-2 text-sm font-medium"
        >
          Try again
        </button>
        <Link to="/" className="text-sm font-medium underline">
          Go to the overview
        </Link>
      </div>
    </Centered>
  );
}
