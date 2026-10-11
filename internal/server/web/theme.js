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
})();
