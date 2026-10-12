import { Card } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
  component: Home,
});

function Home() {
  return (
    <main className="grid min-h-dvh place-items-center p-4">
      <Card className="w-full max-w-sm">
        <Card.Header>
          <Card.Title>ForgeDeck</Card.Title>
          <Card.Description>Self-hosted DevOps control plane for Docker.</Card.Description>
        </Card.Header>
      </Card>
    </main>
  );
}
