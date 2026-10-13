// Follow the OS light/dark setting before first paint to avoid a flash.
// This is a separate file, not an inline script, so the Content-Security-Policy
// can forbid inline scripts altogether.
(() => {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  const apply = () => {
    document.documentElement.dataset.theme = media.matches ? "dark" : "light";
  };
  apply();
  media.addEventListener("change", apply);
})();
