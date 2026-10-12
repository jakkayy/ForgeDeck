import { Button, Card } from "@heroui/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";

const root = document.getElementById("root");
if (!root) {
  throw new Error("Missing #root element");
}

createRoot(root).render(
  <StrictMode>
    <main className="grid min-h-dvh place-items-center p-4">
      <Card className="w-full max-w-sm">
        <Card.Header>
          <Card.Title>ForgeDeck</Card.Title>
          <Card.Description>Self-hosted DevOps control plane for Docker.</Card.Description>
        </Card.Header>
        <Card.Footer>
          <Button variant="primary">Get started</Button>
        </Card.Footer>
      </Card>
    </main>
  </StrictMode>,
);
