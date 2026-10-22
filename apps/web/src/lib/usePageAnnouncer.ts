import { useRouterState } from "@tanstack/react-router";
import { useEffect, useRef } from "react";

const SUFFIX = "ForgeDeck";

/** Moves the document title and keyboard focus to the new page on navigation, for screen readers. */
export function usePageAnnouncer(mainId: string) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const first = useRef(true);

  useEffect(() => {
    const main = document.getElementById(mainId);
    if (!main) {
      return;
    }
    const setTitle = () => {
      const heading = main.querySelector("h1")?.textContent?.trim();
      document.title = heading ? `${heading} · ${SUFFIX}` : SUFFIX;
    };
    setTitle();
    const observer = new MutationObserver(setTitle);
    observer.observe(main, { childList: true, subtree: true, characterData: true });
    return () => observer.disconnect();
  }, [mainId]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: run on every page change
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    document.getElementById(mainId)?.focus({ preventScroll: false });
  }, [pathname, mainId]);
}
