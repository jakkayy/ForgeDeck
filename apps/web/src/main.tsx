import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";

const root = document.getElementById("root");
if (!root) {
  throw new Error("Missing #root element");
}

createRoot(root).render(
  <StrictMode>
    <main className="grid min-h-dvh place-items-center">
      <h1 className="text-2xl font-semibold">ForgeDeck</h1>
    </main>
  </StrictMode>,
);
