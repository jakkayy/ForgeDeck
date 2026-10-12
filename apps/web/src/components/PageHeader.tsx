import type { ReactNode } from "react";

type Props = { title: string; description?: string; actions?: ReactNode };

/** Page title row with an optional primary action on the right. */
export function PageHeader({ title, description, actions }: Props) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="mt-1 text-sm text-[var(--muted)]">{description}</p>}
      </div>
      {actions}
    </div>
  );
}
