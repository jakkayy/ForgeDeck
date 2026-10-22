// Follow the OS light/dark setting before first paint; a file so the CSP can forbid inline scripts.
(() => {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  const apply = () => {
    document.documentElement.dataset.theme = media.matches ? "dark" : "light";
  };
  apply();
  media.addEventListener("change", apply);
})();
