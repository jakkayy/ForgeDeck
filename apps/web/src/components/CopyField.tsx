import { Button } from "@heroui/react";
import { useState } from "react";

type Props = { label: string; value: string };

export function CopyField({ label, value }: Props) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="flex items-start gap-2 rounded-lg bg-[var(--surface-secondary)] p-3">
      <code className="min-w-0 flex-1 break-all font-mono text-xs leading-5">{value}</code>
      <Button size="sm" variant="tertiary" onPress={copy} aria-label={`Copy ${label}`}>
        {copied ? "Copied" : "Copy"}
      </Button>
    </div>
  );
}
