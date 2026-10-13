import { useRouterState } from "@tanstack/react-router";
import { useEffect, useRef } from "react";

const SUFFIX = "ForgeDeck";

/**
 * What a screen reader user needs when the single-page app changes pages,
 * since nothing reloads: the document title follows the page's own heading,
 * and keyboard focus moves to the start of the new page's content (not on
 * the first load, and not for changes that stay on the same page, like tabs).
 */
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
    // Headings often arrive after data does; follow them until the page is left.
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
