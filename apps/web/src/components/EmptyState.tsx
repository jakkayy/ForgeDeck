import { Card } from "@heroui/react";
import type { ReactNode } from "react";

type Props = { title: string; description: string; action?: ReactNode };

/** Explains an empty list and offers the next step. */
export function EmptyState({ title, description, action }: Props) {
  return (
    <Card className="items-center gap-2 px-6 py-12 text-center">
      <p className="font-medium">{title}</p>
      <p className="max-w-md text-sm text-[var(--muted)]">{description}</p>
      {action && <div className="mt-3">{action}</div>}
    </Card>
  );
}
