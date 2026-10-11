"use strict";
// Apply before paint; storage may be unavailable in private browsing.
(() => {
  let saved;
  try {
    saved = localStorage.getItem("mr-graph-theme");
  } catch {}
  document.documentElement.dataset.theme = ["latte", "mocha"].includes(saved)
    ? saved
    : matchMedia("(prefers-color-scheme: dark)").matches
      ? "mocha"
      : "latte";
  const syncFavicon = () => {
    document.querySelector('link[rel="icon"]').href =
      `/mark.svg?theme=${document.documentElement.dataset.theme}`;
  };
  syncFavicon();
  new MutationObserver(syncFavicon).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-theme"],
  });
})();
