"use strict";

const $ = (selector) => document.querySelector(selector);
const token = $('meta[name="mr-graph-session"]').content;
const state = {
  snapshot: null,
  selected: null,
  search: "",
  scale: 1,
  x: 0,
  y: 0,
  fitted: false,
  nextRefresh: Date.now() + 60000,
  requesting: false,
  fingerprint: "",
};
const escapeHTML = (value) =>
  String(value ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const safeURL = (value) => {
  try {
    const u = new URL(value);
    return ["https:", "http:"].includes(u.protocol) ? u.href : "";
  } catch {
    return "";
  }
};
const nodes = () => state.snapshot?.graph.nodes ?? [];
const mrNodes = () => nodes().filter((n) => n.mr);
const edges = () => state.snapshot?.graph.edges ?? [];

function ciSignal(ci) {
  if (ci === "success") return { text: "成功", color: "green" };
  if (["failed", "canceled"].includes(ci))
    return { text: ci === "failed" ? "失敗" : "中止", color: "red" };
  if (
    [
      "running",
      "pending",
      "created",
      "preparing",
      "waiting_for_resource",
      "scheduled",
    ].includes(ci)
  )
    return { text: "進行中", color: "amber" };
  return {
    text:
      {
        loading: "確認中",
        none: "未実行",
        skipped: "スキップ",
        manual: "手動待ち",
      }[ci] ?? "不明",
    color: ci === "manual" ? "amber" : "",
  };
}
function approvalText(a, compact = false) {
  if (a.state === "met")
    return compact
      ? "承認済み"
      : `承認条件を満たしています${a.countKnown ? ` (${a.count} 件)` : ""}`;
  if (a.state === "waiting")
    return Number.isInteger(a.left)
      ? compact
        ? `承認待ち ${a.left}`
        : `あと ${a.left} 件の承認が必要`
      : "承認待ち";
  if (a.state === "not_required") return "必須承認なし";
  if (a.state === "received")
    return compact ? `承認 ${a.count}` : `${a.count} 件の承認 (必須条件は不明)`;
  if (a.state === "loading") return "確認中";
  return "承認条件不明";
}
function relationSets(id) {
  const walk = (reverse) => {
    const seen = new Set([id]),
      queue = [id];
    for (let i = 0; i < queue.length; i++) {
      for (const e of edges()) {
        if ((reverse ? e.target : e.source) === queue[i]) {
          const next = reverse ? e.source : e.target;
          if (!seen.has(next)) {
            seen.add(next);
            queue.push(next);
          }
        }
      }
    }
    return seen;
  };
  return { up: walk(true), down: walk(false) };
}
function relatedIDs() {
  if (!state.selected) return null;
  const { up, down } = relationSets(state.selected);
  return new Set([...up, ...down]);
}
function signalHTML(ci) {
  const s = ciSignal(ci);
  return `<span class="signal"><i class="dot ${s.color}"></i>CI ${escapeHTML(s.text)}</span>`;
}

function renderGraph() {
  const related = relatedIDs(),
    index = new Map(nodes().map((n) => [n.id, n]));
  $("#nodes").innerHTML = nodes()
    .map((n) => {
      const classes = [
        n.mr ? "node" : "node root-node",
        n.kind === "branch" ? "branch" : "",
        state.selected === n.id
          ? "selected"
          : related?.has(n.id)
            ? "related"
            : "",
        related && !related.has(n.id) ? "dim" : "",
      ].join(" ");
      const style = `left:${n.x}px;top:${n.y}px`;
      if (!n.mr)
        return `<div class="${classes}" style="${style}" title="${escapeHTML(n.label)}"><span class="eyebrow">${n.kind === "root" ? "DEFAULT BRANCH" : "BASE BRANCH"}</span><span class="branch-label">${escapeHTML(n.label)}</span></div>`;
      const m = n.mr;
      const conflict =
        m.conflict === "conflict"
          ? '<span class="signal"><i class="dot red"></i>競合</span>'
          : "";
      return `<button class="${classes}" style="${style}" data-id="${escapeHTML(n.id)}" aria-label="!${m.iid} ${escapeHTML(m.title)}" aria-pressed="${state.selected === n.id}"><span class="node-top"><span class="node-number">!${m.iid}</span>${m.draft ? '<span class="draft-tag">DRAFT</span>' : "<span>↗</span>"}</span><span class="node-title" title="${escapeHTML(m.title)}">${escapeHTML(m.title)}</span><span class="node-author"><span class="avatar">${escapeHTML(m.author.slice(0, 1))}</span>${escapeHTML(m.author)}</span><span class="node-status">${signalHTML(m.ci)}${conflict}<span class="signal">${escapeHTML(approvalText(m.approval, true))}</span></span></button>`;
    })
    .join("");
  $("#nodes")
    .querySelectorAll("button[data-id]")
    .forEach((b) =>
      b.addEventListener("click", () => select(b.dataset.id, false)),
    );
  const g = state.snapshot.graph;
  $("#edges").setAttribute("width", g.width);
  $("#edges").setAttribute("height", g.height);
  $("#edges").innerHTML =
    `<defs><marker id="arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M 0 1 L 7 4 L 0 7" fill="none" stroke="#8e9e8b" stroke-width="1.3"/></marker><marker id="active-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M 0 1 L 7 4 L 0 7" fill="none" stroke="#ad422a" stroke-width="1.3"/></marker></defs>` +
    edges()
      .map((e) => {
        const a = index.get(e.source),
          b = index.get(e.target);
        if (!a || !b) return "";
        const x1 = a.x + (a.mr ? 288 : 178),
          y1 = a.y + (a.mr ? 78 : 35),
          x2 = b.x,
          y2 = b.y + (b.mr ? 78 : 35),
          offset = Math.max(28, Math.abs(x2 - x1) / 2);
        const active = related?.has(e.source) && related?.has(e.target);
        const path =
          x2 > x1
            ? `M${x1},${y1} C${x1 + offset},${y1} ${x2 - offset},${y2} ${x2 - 6},${y2}`
            : `M${x1},${y1} C${x1 + 50},${y1 - 75} ${x2 - 50},${y2 - 75} ${x2 - 6},${y2}`;
        return `<path class="edge ${active ? "active" : related ? "dim" : ""} ${e.cycle ? "cycle" : ""}" d="${path}" marker-end="url(#${active ? "active-arrow" : "arrow"})"/>`;
      })
      .join("");
  $("#empty").hidden = nodes().length > 0;
  if (!nodes().length) {
    const loading = state.snapshot.loading;
    $("#empty h2").textContent = loading
      ? "変更を読み込んでいます"
      : state.snapshot.error
        ? "MR を取得できませんでした"
        : "開いている MR はありません";
    $("#empty p").textContent = loading
      ? "MR とブランチの関係を確認しています。"
      : state.snapshot.error
        ? "認証・接続を確認し、更新ボタンで再試行してください。"
        : "新しい MR が作成されると、更新時にここへ表示されます。";
  }
  $("#clear").hidden = !state.selected;
  if (!state.fitted && nodes().length) {
    state.fitted = true;
    fit();
  }
  transform();
}

function renderInspector() {
  const n = nodes().find((n) => n.id === state.selected);
  if (!n?.mr) {
    $("#inspector-content").innerHTML =
      '<div class="inspector-head"><span class="eyebrow">ALL OPEN MERGE REQUESTS</span><h2>チームの変更</h2><input id="search" type="search" aria-label="MR 番号・タイトル・作成者を検索" placeholder="番号・タイトル・作成者で検索"></div><div id="list-count" class="list-count"></div><div id="mr-list" class="mr-list"></div>';
    $("#search").value = state.search;
    $("#search").addEventListener("input", (e) => {
      state.search = e.target.value;
      renderList();
    });
    renderList();
    return;
  }
  const m = n.mr,
    { up, down } = relationSets(n.id),
    ci = ciSignal(m.ci);
  const relationButtons = (ids) =>
    mrNodes()
      .filter((x) => x.id !== n.id && ids.has(x.id))
      .map(
        (x) =>
          `<button class="relation-button" data-relation="${escapeHTML(x.id)}"><span><b>!${x.mr.iid}</b>${escapeHTML(x.mr.title)}</span><span>→</span></button>`,
      )
      .join("");
  const upstream = relationButtons(up),
    downstream = relationButtons(down),
    link = safeURL(m.url);
  const conflictLabel =
    { clear: "競合なし", conflict: "競合あり", loading: "確認中" }[
      m.conflict
    ] ?? "不明";
  $("#inspector-content").innerHTML =
    `<div class="detail"><button id="back" class="back">← MR 一覧に戻る</button><div class="detail-number">MERGE REQUEST !${m.iid}${m.draft ? " · DRAFT" : ""}</div><h2>${escapeHTML(m.title)}</h2><div class="detail-author">by @${escapeHTML(m.author)}</div>${link ? `<a class="external-link" href="${escapeHTML(link)}" target="_blank" rel="noopener noreferrer">GitLab でレビューする<span>↗</span></a>` : ""}<section class="detail-section"><h3>BRANCHES</h3><div class="branch-flow">${escapeHTML(m.targetBranch)}<br><b>↳</b> ${escapeHTML(m.sourceBranch)}</div></section><section class="detail-section"><h3>STATUS</h3><div class="status-row"><span>Draft</span><span>${m.draft ? "レビュー準備中" : "レビュー可能"}</span></div><div class="status-row"><span>CI</span><span class="status-label"><i class="dot ${ci.color}"></i>${escapeHTML(ci.text)}</span></div><div class="status-row"><span>競合</span><span>${conflictLabel}</span></div><div class="status-row"><span>承認</span><span>${escapeHTML(approvalText(m.approval))}</span></div><p class="detail-note">最終的なマージ条件は GitLab で確認してください。</p>${(m.warnings ?? []).map((w) => `<div class="detail-warning">${escapeHTML(w)}</div>`).join("")}</section><section class="detail-section"><h3>UPSTREAM · 前提となる MR</h3>${upstream || '<p class="detail-note">表示対象内に上流 MR はありません。</p>'}</section><section class="detail-section"><h3>DOWNSTREAM · この上に積まれた MR</h3>${downstream || '<p class="detail-note">表示対象内に下流 MR はありません。</p>'}</section></div>`;
  $("#back").addEventListener("click", clear);
  $("#inspector-content")
    .querySelectorAll("[data-relation]")
    .forEach((b) =>
      b.addEventListener("click", () => select(b.dataset.relation, true)),
    );
}
function renderList() {
  const query = state.search.trim().toLocaleLowerCase();
  const list = mrNodes().filter((n) =>
    `${n.mr.iid} ${n.mr.title} ${n.mr.author} ${n.mr.sourceBranch}`
      .toLocaleLowerCase()
      .includes(query),
  );
  $("#list-count").textContent =
    `${list.length} / ${mrNodes().length} MERGE REQUESTS`;
  $("#mr-list").innerHTML =
    list
      .map(
        (n) =>
          `<button class="mr-list-item" data-list-id="${escapeHTML(n.id)}"><span class="list-meta"><span class="node-number">!${n.mr.iid}</span><i class="dot ${ciSignal(n.mr.ci).color}"></i><span>@${escapeHTML(n.mr.author)}</span>${n.mr.draft ? '<span class="draft-tag">DRAFT</span>' : ""}</span><span class="list-title">${escapeHTML(n.mr.title)}</span><span class="list-branch">${escapeHTML(n.mr.targetBranch)} ← ${escapeHTML(n.mr.sourceBranch)}</span></button>`,
      )
      .join("") || '<p class="list-empty">一致する MR はありません。</p>';
  $("#mr-list")
    .querySelectorAll("[data-list-id]")
    .forEach((b) =>
      b.addEventListener("click", () => select(b.dataset.listId, true)),
    );
}
function select(id, focus) {
  state.selected = id;
  renderGraph();
  renderInspector();
  $("#inspector").scrollTop = 0;
  if (focus) focusNode(id);
  if (innerWidth <= 760) $("#inspector").classList.add("mobile-open");
}
function clear() {
  state.selected = null;
  renderGraph();
  renderInspector();
}
function transform() {
  $("#world").style.transform =
    `translate(${state.x}px,${state.y}px) scale(${state.scale})`;
  $("#zoom-label").textContent = `${Math.round(state.scale * 100)}%`;
}
function fit() {
  if (!state.snapshot) return;
  const c = $("#canvas"),
    g = state.snapshot.graph;
  state.scale = Math.max(
    0.035,
    Math.min(
      1,
      (c.clientWidth - 72) / g.width,
      (c.clientHeight - 90) / g.height,
    ),
  );
  state.x = (c.clientWidth - g.width * state.scale) / 2;
  state.y = (c.clientHeight - g.height * state.scale) / 2;
  transform();
}
function focusNode(id) {
  const n = nodes().find((n) => n.id === id);
  if (!n) return;
  const c = $("#canvas");
  const { up, down } = relationSets(id);
  const context = mrNodes().filter(
    (node) => up.has(node.id) || down.has(node.id),
  );
  const left = Math.min(...context.map((node) => node.x));
  const top = Math.min(...context.map((node) => node.y));
  const width = Math.max(...context.map((node) => node.x)) - left + 288;
  const height = Math.max(...context.map((node) => node.y)) - top + 156;
  const contextScale = Math.min(
    1,
    (c.clientWidth - 80) / width,
    (c.clientHeight - 100) / height,
  );
  // Fit the whole stack when labels remain readable. Large stacks retain a
  // readable scale centered on the selected MR, with all relations in the panel.
  if (contextScale >= 0.6) {
    state.scale = contextScale;
    state.x = (c.clientWidth - width * state.scale) / 2 - left * state.scale;
    state.y = (c.clientHeight - height * state.scale) / 2 - top * state.scale;
  } else {
    state.scale = Math.max(0.8, state.scale);
    state.x = c.clientWidth / 2 - (n.x + 144) * state.scale;
    state.y = c.clientHeight / 2 - (n.y + 78) * state.scale;
  }
  transform();
}
function zoom(
  factor,
  x = $("#canvas").clientWidth / 2,
  y = $("#canvas").clientHeight / 2,
) {
  const next = Math.max(0.035, Math.min(1.8, state.scale * factor));
  const ratio = next / state.scale;
  state.x = x - (x - state.x) * ratio;
  state.y = y - (y - state.y) * ratio;
  state.scale = next;
  transform();
}

async function request(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: { "X-MR-Graph-Session": token },
  });
  if (!response.ok)
    throw new Error(
      response.status === 403
        ? "ページを再読み込みしてください。"
        : `接続エラー (${response.status})`,
    );
  return response;
}
async function poll() {
  try {
    const snapshot = await (await request("/api/graph")).json();
    state.snapshot = snapshot;
    if (
      state.selected &&
      !snapshot.graph.nodes.some((n) => n.id === state.selected)
    )
      state.selected = null;
    const fingerprint = JSON.stringify(snapshot.graph);
    if (fingerprint !== state.fingerprint) {
      state.fingerprint = fingerprint;
      const active = document.activeElement;
      const position = active?.id === "search" ? active.selectionStart : null;
      const focusedNode = active?.dataset.id,
        focusedRelation = active?.dataset.relation,
        focusedList = active?.dataset.listId;
      const focusedElement = active?.id,
        focusedLink = active?.classList.contains("external-link");
      const scrollPosition = $("#inspector").scrollTop;
      renderGraph();
      renderInspector();
      $("#inspector").scrollTop = scrollPosition;
      if (position !== null) {
        $("#search")?.focus({ preventScroll: true });
        $("#search")?.setSelectionRange(position, position);
      } else {
        const focusTarget = focusedNode
          ? [...document.querySelectorAll(".node[data-id]")].find(
              (n) => n.dataset.id === focusedNode,
            )
          : focusedRelation
            ? [...document.querySelectorAll("[data-relation]")].find(
                (n) => n.dataset.relation === focusedRelation,
              )
            : focusedList
              ? [...document.querySelectorAll("[data-list-id]")].find(
                  (n) => n.dataset.listId === focusedList,
                )
              : focusedLink
                ? $(".external-link")
                : focusedElement
                  ? document.getElementById(focusedElement)
                  : null;
        focusTarget?.focus({ preventScroll: true });
      }
    }
    $("#project-link").textContent = snapshot.graph.project.path;
    const projectURL = safeURL(snapshot.graph.project.url);
    if (projectURL) $("#project-link").href = projectURL;
    $("#count").textContent = mrNodes().length;
    $("#demo-banner").hidden = !snapshot.demo;
    $("#version").textContent = snapshot.version;
    $("#update-status").textContent = snapshot.update;
    $("#update-status").title = snapshot.update;
    $("#error-banner").hidden =
      !snapshot.error && !snapshot.graph.warnings.length;
    $("#error-banner").textContent =
      snapshot.error ||
      (snapshot.graph.warnings.length
        ? `${snapshot.graph.warnings.slice(0, 2).join(" / ")}${snapshot.graph.warnings.length > 2 ? ` / ほか ${snapshot.graph.warnings.length - 2} 件` : ""}`
        : "");
    $("#error-banner").classList.toggle("warning", !snapshot.error);
    $("#refresh").disabled = snapshot.loading;
    $("#refresh").textContent = snapshot.loading ? "↻ 確認中" : "↻ 更新";
    const pending = mrNodes().filter(
      (n) => n.mr.ci === "loading" || n.mr.approval.state === "loading",
    ).length;
    $("#connection").innerHTML =
      `<i class="dot ${snapshot.error ? "red" : snapshot.loading ? "amber" : "green"}"></i>${snapshot.loading ? (pending ? `状態を確認中 ${mrNodes().length - pending}/${mrNodes().length}` : "MR を確認中") : snapshot.demo ? "デモ表示" : "接続済み"}`;
    $("#updated").textContent = snapshot.updatedAt
      ? `更新 ${new Date(snapshot.updatedAt).toLocaleTimeString("ja-JP", { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`
      : "まだ更新されていません";
    // Empty graphs can change loading state without changing the graph payload.
    if (!nodes().length) renderGraph();
  } catch (error) {
    $("#error-banner").hidden = false;
    $("#error-banner").textContent = error.message;
    $("#connection").textContent = "ローカルサーバーに接続できません";
  }
  setTimeout(poll, state.snapshot?.loading ? 600 : 3000);
}
async function refresh() {
  if (state.requesting) return;
  state.requesting = true;
  state.nextRefresh = Date.now() + 60000;
  try {
    await request("/api/refresh", { method: "POST" });
  } catch (error) {
    $("#error-banner").hidden = false;
    $("#error-banner").textContent = error.message;
  } finally {
    state.requesting = false;
  }
}

$("#refresh").addEventListener("click", refresh);
$("#clear").addEventListener("click", clear);
$("#fit").addEventListener("click", fit);
$("#zoom-in").addEventListener("click", () => zoom(1.25));
$("#zoom-out").addEventListener("click", () => zoom(0.8));
$("#sidebar-toggle").addEventListener("click", () =>
  $("#inspector").classList.toggle("mobile-open"),
);
$("#auto").addEventListener("change", () => {
  state.nextRefresh = Date.now() + 60000;
});
setInterval(() => {
  if ($("#auto").checked && !document.hidden && Date.now() >= state.nextRefresh)
    refresh();
}, 1000);
document.addEventListener("visibilitychange", () => {
  if (!document.hidden && $("#auto").checked && Date.now() >= state.nextRefresh)
    refresh();
});
const canvas = $("#canvas");
let drag = null;
canvas.addEventListener("pointerdown", (e) => {
  if (e.target.closest("button")) return;
  drag = { x: e.clientX, y: e.clientY, startX: state.x, startY: state.y };
  canvas.setPointerCapture(e.pointerId);
  canvas.classList.add("dragging");
});
canvas.addEventListener("pointermove", (e) => {
  if (!drag) return;
  state.x = drag.startX + e.clientX - drag.x;
  state.y = drag.startY + e.clientY - drag.y;
  transform();
});
const stopDrag = () => {
  drag = null;
  canvas.classList.remove("dragging");
};
canvas.addEventListener("pointerup", stopDrag);
canvas.addEventListener("pointercancel", stopDrag);
canvas.addEventListener(
  "wheel",
  (e) => {
    if (e.target.closest(".zoom-tools")) return;
    e.preventDefault();
    const r = canvas.getBoundingClientRect();
    zoom(Math.exp(-e.deltaY * 0.0015), e.clientX - r.left, e.clientY - r.top);
  },
  { passive: false },
);
canvas.addEventListener("keydown", (e) => {
  if (e.target.closest("button")) return;
  const moves = {
    ArrowLeft: [40, 0],
    ArrowRight: [-40, 0],
    ArrowUp: [0, 40],
    ArrowDown: [0, -40],
  };
  if (moves[e.key]) {
    e.preventDefault();
    state.x += moves[e.key][0];
    state.y += moves[e.key][1];
    transform();
  }
  if (e.key === "+" || e.key === "=") zoom(1.25);
  if (e.key === "-") zoom(0.8);
  if (e.key === "0") fit();
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    clear();
    $("#inspector").classList.remove("mobile-open");
  }
});
new ResizeObserver(() => {
  if (state.fitted) transform();
}).observe(canvas);
renderInspector();
poll();
