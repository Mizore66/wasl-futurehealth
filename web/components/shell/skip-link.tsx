/** First focusable element on every page with a persistent `<main id="main">`. Visible only while focused. */
export function SkipLink() {
  return (
    <a
      href="#main"
      className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-50 focus:rounded-control focus:bg-ink focus:px-4 focus:py-3 focus:text-body focus:text-ground focus:no-underline"
    >
      Skip to main content
    </a>
  );
}
