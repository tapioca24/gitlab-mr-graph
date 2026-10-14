"use strict";

const $ = (selector) => document.querySelector(selector);
const token = $('meta[name="mr-graph-session"]').content;
const state = {
  snapshot: null,
  selected: null,
  scale: 1,
  x: 0,
  y: 0,
  fitted: false,
  nextRefresh: Date.now() + 60000,
  requesting: false,
  fingerprint: "",
  filters: {
    authored: false,
    assigned: false,
    hideBots: false,
    approvalWaiting: false,
    labels: [],
    query: "",
  },
  visible: new Set(),
  matches: new Set(),
  positions: new Map(),
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
const icon = (name) =>
  `<i class="icon" style="--icon:url('/icons/${name}.svg')" aria-hidden="true"></i>`;
const author = (m) => ({
  ...m.authorUser,
  username: m.authorUser?.username || m.author,
});

function avatar(user, prefix = "") {
  const name = user.name || user.username || "不明";
  const url = safeURL(user.avatarUrl);
  const label = `${prefix}${name}${user.username ? ` (@${user.username})` : ""}`;
  return `<span class="avatar" title="${escapeHTML(label)}" role="img" aria-label="${escapeHTML(label)}"><span>${escapeHTML(name.slice(0, 1))}</span>${url ? `<img src="${escapeHTML(url)}" alt="" loading="lazy" referrerpolicy="no-referrer" />` : ""}</span>`;
}
function labelsHTML(m, compact = false) {
  const labels = m.labels ?? [];
  const shown = compact ? labels.slice(0, 2) : labels;
  const safeColor = (value) =>
    /^#[0-9a-f]{6}$/i.test(value ?? "") ? value : "";
  const textColor = (color) => {
    const [r, g, b] = [1, 3, 5].map((i) => parseInt(color.slice(i, i + 2), 16));
    return (r * 299 + g * 587 + b * 114) / 1000 > 145 ? "#000000" : "#ffffff";
  };
  return `<span class="labels">${shown
    .map((l) => {
      const bg = safeColor(l.color),
        fg = safeColor(l.textColor) || (bg ? textColor(bg) : "");
      return `<span class="mr-label" title="${escapeHTML(l.name)}"${bg ? ` style="background:${bg};color:${fg}"` : ""}>${escapeHTML(l.name)}</span>`;
    })
    .join("")}${
    compact && labels.length > shown.length
      ? `<span class="avatar-more" title="${escapeHTML(
          labels
            .slice(shown.length)
            .map((l) => l.name)
            .join(", "),
        )}">+${labels.length - shown.length}</span>`
      : ""
  }</span>`;
}
function approversHTML(m) {
  const people = m.approval.users ?? [];
  if (!people.length) return "";
  return `<span class="approvers">${icon("check")}<span class="avatar-stack">${people
    .slice(0, 4)
    .map((u) => avatar(u, "承認: "))
    .join("")}</span>${
    people.length > 4
      ? `<span class="avatar-more" title="${escapeHTML(
          people
            .slice(4)
            .map((u) => u.name || u.username)
            .join(", "),
        )}">+${people.length - 4}</span>`
      : ""
  }</span>`;
}
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
  if (a.state === "unapproved") return "未承認";
  if (a.state === "received")
    return `承認 ${a.count}件（必須条件不明）`;
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

const nodeElements = new Map(),
  edgeElements = new Map();
function renderGraph() {
  if (!state.snapshot) return;
  const g = state.snapshot.graph;
  const { matches, visible } = graphVisibility(g, state.filters);
  state.matches = matches;
  state.visible = visible;
  const positions = layoutVisibleGraph(g, visible);
  state.positions = positions;
  if (state.selected && !visible.has(state.selected)) state.selected = null;
  const related = relatedIDs();
  for (const [id, element] of nodeElements) {
    if (!nodes().some((n) => n.id === id)) {
      element.remove();
      nodeElements.delete(id);
    }
  }
  for (const n of nodes()) {
    let element = nodeElements.get(n.id);
    const fresh = !element;
    if (fresh) {
      element = document.createElement("article");
      element.dataset.id = n.id;
      nodeElements.set(n.id, element);
      $("#nodes").append(element);
    }
    const contentKey = JSON.stringify(n.mr ?? [n.kind, n.label]);
    if (element.contentKey !== contentKey) {
      element.contentKey = contentKey;
      if (!n.mr) {
        element.innerHTML = `<span class="eyebrow">${n.kind === "root" ? "DEFAULT BRANCH" : "BASE BRANCH"}</span><span class="branch-label" title="${escapeHTML(n.label)}">${escapeHTML(n.label)}</span>`;
      } else {
        const m = n.mr,
          link = safeURL(m.url);
        const conflict =
          m.conflict === "conflict"
            ? '<span class="signal"><i class="dot red"></i>競合</span>'
            : "";
        element.innerHTML = `<button class="node-select" aria-label="!${m.iid} ${escapeHTML(m.title)}"><span class="node-top"><span class="node-number">!${m.iid}</span>${m.draft ? '<span class="draft-tag">DRAFT</span>' : ""}</span><span class="node-title" title="${escapeHTML(m.title)}">${escapeHTML(m.title)}</span><span class="node-people"><span class="node-author">${avatar(author(m))}<span>${escapeHTML(m.author)}</span></span>${approversHTML(m)}</span>${labelsHTML(m, true)}<span class="node-status">${signalHTML(m.ci)}${conflict}<span class="signal">${escapeHTML(approvalText(m.approval, true))}</span></span></button>${link ? `<a class="node-link icon-button" href="${escapeHTML(link)}" target="_blank" rel="noopener noreferrer" aria-label="!${m.iid} を GitLab で開く" title="GitLab で開く">${icon("external-link")}</a>` : ""}`;
      }
    }
    element.className = [
      "node",
      !n.mr ? "root-node" : "",
      n.kind === "branch" ? "branch" : "",
      state.selected === n.id
        ? "selected"
        : related?.has(n.id)
          ? "related"
          : "",
      n.mr && !matches.has(n.id) ? "context" : "",
      !visible.has(n.id) ? "filtered-out" : "",
    ].join(" ");
    element.inert = !visible.has(n.id);
    element.setAttribute("aria-hidden", String(!visible.has(n.id)));
    element
      .querySelector(".node-select")
      ?.setAttribute("aria-pressed", String(state.selected === n.id));
    const p = positions.get(n.id);
    if (p) {
      element.style.left = `${p.x}px`;
      element.style.top = `${p.y}px`;
    } else if (fresh) {
      element.style.left = `${n.x}px`;
      element.style.top = `${n.y}px`;
    }
    if (
      fresh &&
      visible.has(n.id) &&
      !matchMedia("(prefers-reduced-motion: reduce)").matches
    ) {
      element.animate(
        [{ opacity: 0 }, { opacity: matches.has(n.id) || !n.mr ? 1 : 0.38 }],
        { duration: 240 },
      );
    }
  }
  const index = new Map(nodes().map((n) => [n.id, n]));
  for (const [id, element] of edgeElements) {
    if (!edges().some((e) => e.id === id)) {
      element.remove();
      edgeElements.delete(id);
    }
  }
  for (const e of edges()) {
    let element = edgeElements.get(e.id);
    if (!element) {
      element = document.createElementNS("http://www.w3.org/2000/svg", "path");
      edgeElements.set(e.id, element);
      $("#edges").append(element);
    }
    const shown = visible.has(e.source) && visible.has(e.target);
    const a = index.get(e.source),
      b = index.get(e.target);
    const pa = positions.get(e.source),
      pb = positions.get(e.target);
    if (a && b && pa && pb) {
      const x1 = pa.x + (a.mr ? 288 : 178),
        y1 = pa.y + (a.mr ? 95 : 35);
      const x2 = pb.x,
        y2 = pb.y + (b.mr ? 95 : 35);
      const offset = Math.max(28, Math.abs(x2 - x1) / 2);
      const path =
        x2 > x1
          ? `M${x1},${y1} C${x1 + offset},${y1} ${x2 - offset},${y2} ${x2},${y2}`
          : `M${x1},${y1} C${x1 + 50},${y1 - 85} ${x2 - 50},${y2 - 85} ${x2},${y2}`;
      element.setAttribute("d", path);
      element.style.d = `path("${path}")`;
    }
    element.setAttribute(
      "class",
      [
        "edge",
        related?.has(e.source) && related?.has(e.target) ? "active" : "",
        !matches.has(e.target) ? "context" : "",
        e.cycle ? "cycle" : "",
        !shown ? "filtered-out" : "",
      ].join(" "),
    );
  }
  $("#count").textContent = `${matches.size} / ${mrNodes().length} MR`;
  $("#count").title =
    `一致: ${matches.size} 件 · 経路として表示: ${[...visible].filter((id) => index.get(id)?.mr && !matches.has(id)).length} 件`;
  $("#empty").hidden = matches.size > 0;
  if (!matches.size) {
    const loading = state.snapshot.loading;
    $("#empty h2").textContent = mrNodes().length
      ? "条件に一致する MR はありません"
      : loading
        ? "変更を読み込んでいます"
        : state.snapshot.error
          ? "MR を取得できませんでした"
          : "開いている MR はありません";
    $("#empty p").textContent = mrNodes().length
      ? "Query やチェックボックスの条件を変えてください。"
      : loading
        ? "MR とブランチの関係を確認しています。"
        : state.snapshot.error
          ? "認証・接続を確認し、更新ボタンで再試行してください。"
          : "新しい MR は更新時にここへ表示されます。";
  }
  if (!state.fitted && visible.size) {
    state.fitted = true;
    fit(false);
  }
  transform();
}

function personHTML(u) {
  return `<div class="detail-author">${avatar(u)}<span class="person-name">${escapeHTML(u.name || u.username)}<small>@${escapeHTML(u.username)}</small></span></div>`;
}
function peopleHTML(m) {
  return `<div class="detail-people"><div class="person-role"><h3>Author</h3><div class="people-list">${personHTML(author(m))}</div></div><div class="person-role"><h3>Assigned</h3><div class="people-list">${(m.assignees ?? []).length ? m.assignees.map(personHTML).join("") : '<span class="detail-note">未設定</span>'}</div></div></div>`;
}
function renderInspector(animate = false) {
  const n = nodes().find((n) => n.id === state.selected),
    panel = $("#inspector");
  const open = !!n?.mr;
  panel.classList.toggle("open", open);
  panel.inert = !open;
  panel.setAttribute("aria-hidden", String(!open));
  if (!open) return;
  const m = n.mr,
    { up, down } = relationSets(n.id),
    ci = ciSignal(m.ci);
  const relationButtons = (ids) =>
    mrNodes()
      .filter((x) => x.id !== n.id && ids.has(x.id))
      .map(
        (x) =>
          `<button class="relation-button" data-relation="${escapeHTML(x.id)}"${!state.visible.has(x.id) ? ' title="絞り込みを解除してこの MR を表示"' : ""}><b>!${x.mr.iid}</b><span class="relation-title">${escapeHTML(x.mr.title)}</span>${icon("chevron-right")}</button>`,
      )
      .join("");
  const link = safeURL(m.url);
  const conflictLabel =
    { clear: "競合なし", conflict: "競合あり", loading: "確認中" }[
      m.conflict
    ] ?? "不明";
  const html = `<div class="detail"><div class="detail-top"><span class="detail-number">MERGE REQUEST !${m.iid}</span>${m.draft ? '<span class="draft-tag">DRAFT</span>' : ""}<button id="close-detail" class="icon-button" aria-label="MR 詳細を閉じる" title="閉じる (Esc)">${icon("x")}</button></div><h2>${escapeHTML(m.title)}</h2>${peopleHTML(m)}${labelsHTML(m)}${link ? `<a class="external-link" href="${escapeHTML(link)}" target="_blank" rel="noopener noreferrer">GitLab で開く${icon("external-link")}</a>` : ""}<section class="detail-section"><h3>${icon("git-branch")}BRANCHES</h3><div class="branch-flow"><span>${escapeHTML(m.targetBranch)}</span><span>${icon("arrow-right")} ${escapeHTML(m.sourceBranch)}</span></div></section><section class="detail-section"><h3>STATUS</h3><div class="status-row"><span>CI</span><span class="status-label"><i class="dot ${ci.color}"></i>${escapeHTML(ci.text)}</span></div><div class="status-row"><span>競合</span><span>${conflictLabel}</span></div><div class="status-row"><span>承認</span><span>${escapeHTML(approvalText(m.approval))}</span></div>${(m.approval.users ?? []).length ? `<div class="people-list">${m.approval.users.map(personHTML).join("")}</div>` : ""}${(m.warnings ?? []).map((w) => `<div class="detail-warning">${escapeHTML(w)}</div>`).join("")}</section><section class="detail-section"><h3>UPSTREAM · 前提となる MR</h3>${relationButtons(up) || '<p class="detail-note">上流 MR はありません。</p>'}</section><section class="detail-section"><h3>DOWNSTREAM · この上に積まれた MR</h3>${relationButtons(down) || '<p class="detail-note">下流 MR はありません。</p>'}</section></div>`;
  if ($("#inspector-content").contentKey === html) return;
  const focused = document.activeElement;
  const focusedInside = panel.contains(focused);
  const focusedExternal = focused?.classList.contains("external-link");
  const focusID = focused?.id,
    relationID = focused?.dataset.relation;
  const scrollTop = panel.scrollTop;
  $("#inspector-content").innerHTML = html;
  $("#inspector-content").contentKey = html;
  if (!animate) $(".detail").style.animation = "none";
  panel.scrollTop = animate ? 0 : scrollTop;
  if (focusedInside) {
    const target = relationID
      ? [...panel.querySelectorAll("[data-relation]")].find(
          (b) => b.dataset.relation === relationID,
        )
      : focusedExternal
        ? $(".external-link")
        : document.getElementById(focusID);
    (target ?? $("#close-detail"))?.focus({ preventScroll: true });
  }
}
function select(id, focus = true) {
  const n = nodes().find((n) => n.id === id);
  if (!n?.mr) return;
  // A relation may be hidden by the filters. Reveal the selected MR by clearing
  // conditions, so the panel never selects an invisible node.
  if (!state.visible.has(id)) resetFilters();
  state.selected = id;
  renderGraph();
  renderInspector(true);
  if (focus) focusNode(id);
}
function clear(returnFocus = false) {
  const id = state.selected;
  state.selected = null;
  renderGraph();
  renderInspector();
  if (returnFocus)
    nodeElements
      .get(id)
      ?.querySelector(".node-select")
      ?.focus({ preventScroll: true });
}
function resetFilters() {
  state.filters = {
    authored: false,
    assigned: false,
    hideBots: false,
    approvalWaiting: false,
    labels: [],
    query: "",
  };
  for (const [id, key] of [
    ["authored", "authored"],
    ["assigned", "assigned"],
    ["hide-bots", "hideBots"],
    ["approval-waiting", "approvalWaiting"],
  ])
    $(`#${id}`).checked = state.filters[key];
  $("#query").value = "";
  renderLabelFilters();
}
let labelOptionsKey = "";
function renderLabelFilters() {
  // Keep selected labels available even if they disappear on refresh.
  const names = [...new Set([
    ...mrNodes().flatMap((n) => (n.mr.labels ?? []).map((label) => label.name)),
    ...state.filters.labels,
  ])].sort((a, b) => a.localeCompare(b));
  const key = JSON.stringify(names);
  if (key !== labelOptionsKey) {
    labelOptionsKey = key;
    $("#label-options").innerHTML = names.length
      ? names.map((name) => `<label class="filter-check label-option"><input type="checkbox" value="${escapeHTML(name)}" />${escapeHTML(name)}</label>`).join("")
      : '<p class="label-filter-empty">No labels</p>';
  }
  for (const input of $("#label-options").querySelectorAll("input"))
    input.checked = state.filters.labels.includes(input.value);
  const count = state.filters.labels.length;
  $("#label-filter-summary").textContent = count ? `Labels (${count})` : "Labels";
  $("#label-filter-summary").title = `Labels を開閉 (G)${count ? ` · ${state.filters.labels.join(" / ")}` : ""}`;
  $("#label-filter").classList.toggle("active", count > 0);
  $("#clear-labels").disabled = count === 0;
}
function applyFilters() {
  renderGraph();
  renderInspector();
  if (state.visible.size) fit();
}

let animationTimer;
let gridFrame;
function updateGrid(x, y, scale) {
  $("#canvas").style.backgroundSize = `${20 * scale}px ${20 * scale}px`;
  $("#canvas").style.backgroundPosition = `${x}px ${y}px`;
  $("#canvas").style.setProperty("--grid-dot", `${0.7 * scale}px`);
}
function followGrid() {
  const world = $("#world");
  const matrix = new DOMMatrixReadOnly(getComputedStyle(world).transform);
  updateGrid(matrix.e, matrix.f, matrix.a);
  if (world.classList.contains("animating"))
    gridFrame = requestAnimationFrame(followGrid);
}
function transform(animate = false) {
  if (animate) {
    $("#world").classList.add("animating");
    clearTimeout(animationTimer);
    animationTimer = setTimeout(
      () => $("#world").classList.remove("animating"),
      340,
    );
  }
  $("#world").style.transform =
    `translate(${state.x}px,${state.y}px) scale(${state.scale})`;
  cancelAnimationFrame(gridFrame);
  if ($("#world").classList.contains("animating")) followGrid();
  else updateGrid(state.x, state.y, state.scale);
  $("#zoom-label").textContent = `${Math.round(state.scale * 100)}%`;
}
function viewport() {
  const canvas = $("#canvas"),
    pane = state.selected ? $("#inspector").getBoundingClientRect().width : 0;
  return {
    width: Math.max(80, canvas.clientWidth - pane),
    height: canvas.clientHeight,
  };
}
function bounds(ids = state.visible) {
  const list = nodes().filter(
    (n) => ids.has(n.id) && state.positions.has(n.id),
  );
  if (!list.length) return null;
  const left = Math.min(...list.map((n) => state.positions.get(n.id).x));
  const top = Math.min(...list.map((n) => state.positions.get(n.id).y));
  return {
    left,
    top,
    width:
      Math.max(
        ...list.map((n) => state.positions.get(n.id).x + (n.mr ? 288 : 178)),
      ) - left,
    height:
      Math.max(
        ...list.map((n) => state.positions.get(n.id).y + (n.mr ? 190 : 70)),
      ) - top,
  };
}
function fit(animate = true) {
  const b = bounds();
  if (!b) return;
  const c = viewport();
  state.scale = Math.max(
    0.035,
    Math.min(1, (c.width - 72) / b.width, (c.height - 90) / b.height),
  );
  state.x = (c.width - b.width * state.scale) / 2 - b.left * state.scale;
  state.y = (c.height - b.height * state.scale) / 2 - b.top * state.scale;
  transform(animate);
}
function focusNode(id) {
  const p = state.positions.get(id);
  if (!p) return;
  const c = viewport(),
    { up, down } = relationSets(id);
  const b = bounds(
    new Set([...up, ...down].filter((x) => state.visible.has(x))),
  );
  const contextScale = b
    ? Math.min(1, (c.width - 72) / b.width, (c.height - 90) / b.height)
    : 0;
  if (contextScale >= 0.6) {
    state.scale = contextScale;
    state.x = (c.width - b.width * state.scale) / 2 - b.left * state.scale;
    state.y = (c.height - b.height * state.scale) / 2 - b.top * state.scale;
  } else {
    state.scale = Math.min(1, Math.max(0.6, (c.width - 32) / 288));
    state.x = c.width / 2 - (p.x + 144) * state.scale;
    state.y = c.height / 2 - (p.y + 95) * state.scale;
  }
  transform(true);
}
function zoom(
  factor,
  x = viewport().width / 2,
  y = viewport().height / 2,
  animate = false,
) {
  const next = Math.max(0.035, Math.min(1.8, state.scale * factor)),
    ratio = next / state.scale;
  state.x = x - (x - state.x) * ratio;
  state.y = y - (y - state.y) * ratio;
  state.scale = next;
  transform(animate);
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
    const fingerprint = JSON.stringify(snapshot.graph);
    if (fingerprint !== state.fingerprint) {
      state.fingerprint = fingerprint;
      const focusNodeID = document.activeElement?.closest(".node")?.dataset.id;
      const focusLink = document.activeElement?.classList.contains("node-link");
      renderLabelFilters();
      renderGraph();
      renderInspector();
      if (focusNodeID)
        nodeElements
          .get(focusNodeID)
          ?.querySelector(focusLink ? ".node-link" : ".node-select")
          ?.focus({ preventScroll: true });
    }
    $("#project-link").textContent = snapshot.graph.project.path;
    const projectURL = safeURL(snapshot.graph.project.url);
    if (projectURL) $("#project-link").href = projectURL;
    $("#demo-banner").hidden = !snapshot.demo;
    for (const id of ["authored", "assigned"]) {
      $(`#${id}`).disabled = !snapshot.graph.project.currentUser;
    }
    $("#error-banner").hidden =
      !snapshot.error && !snapshot.graph.warnings.length;
    $("#error-banner").textContent =
      snapshot.error ||
      snapshot.graph.warnings.slice(0, 2).join(" / ") +
        (snapshot.graph.warnings.length > 2
          ? ` / ほか ${snapshot.graph.warnings.length - 2} 件`
          : "");
    $("#error-banner").classList.toggle("warning", !snapshot.error);
    $("#refresh").disabled = snapshot.loading || state.requesting;
    $("#refresh").classList.toggle("loading", snapshot.loading);
    $("#refresh").setAttribute(
      "aria-label",
      snapshot.loading ? "更新中" : "更新",
    );
    const updated = snapshot.updatedAt
      ? `更新 ${new Date(snapshot.updatedAt).toLocaleTimeString("ja-JP")}`
      : "まだ更新されていません";
    const status = snapshot.error
      ? "接続エラー"
      : snapshot.loading
        ? "更新中"
        : snapshot.demo
          ? "デモ表示"
          : "接続済み";
    $("#connection").innerHTML =
      `<i class="dot ${snapshot.error ? "red" : snapshot.loading ? "amber" : "green"}"></i>`;
    $("#connection").title =
      `${status} · ${updated}\n${snapshot.version}`;
    $("#connection").setAttribute("aria-label", `${status} · ${updated}`);
    $("#refresh").title =
      `${snapshot.loading ? "更新中" : "更新"} (R) · ${updated}`;
    if (!mrNodes().length) renderGraph();
  } catch (error) {
    $("#error-banner").hidden = false;
    $("#error-banner").textContent = error.message;
    $("#connection").setAttribute(
      "aria-label",
      "ローカルサーバーに接続できません",
    );
    $("#connection").title = "ローカルサーバーに接続できません";
    $("#connection").innerHTML = '<i class="dot red"></i>';
  }
  setTimeout(poll, state.snapshot?.loading ? 600 : 3000);
}
async function refresh() {
  if (state.requesting || state.snapshot?.loading) return;
  state.requesting = true;
  $("#refresh").disabled = true;
  state.nextRefresh = Date.now() + 60000;
  try {
    await request("/api/refresh", { method: "POST" });
  } catch (error) {
    $("#error-banner").hidden = false;
    $("#error-banner").textContent = error.message;
  } finally {
    state.requesting = false;
    $("#refresh").disabled = !!state.snapshot?.loading;
  }
}

$("#nodes").addEventListener(
  "error",
  (e) => {
    if (e.target.tagName === "IMG") e.target.hidden = true;
  },
  true,
);
$("#inspector-content").addEventListener(
  "error",
  (e) => {
    if (e.target.tagName === "IMG") e.target.hidden = true;
  },
  true,
);
$("#inspector-content").addEventListener("click", (e) => {
  if (e.target.closest("#close-detail")) clear(true);
  const relation = e.target.closest("[data-relation]");
  if (relation) select(relation.dataset.relation);
});
for (const [id, key] of [
  ["authored", "authored"],
  ["assigned", "assigned"],
  ["hide-bots", "hideBots"],
  ["approval-waiting", "approvalWaiting"],
])
  $(`#${id}`).addEventListener("change", (e) => {
    state.filters[key] = e.target.checked;
    applyFilters();
  });
$("#label-options").addEventListener("change", () => {
  state.filters.labels = [...$("#label-options").querySelectorAll("input:checked")]
    .map((input) => input.value);
  renderLabelFilters();
  applyFilters();
});
$("#clear-labels").addEventListener("click", () => {
  state.filters.labels = [];
  renderLabelFilters();
  applyFilters();
});
document.addEventListener("click", (e) => {
  if (!e.target.closest("#label-filter")) $("#label-filter").open = false;
});
$("#label-filter").addEventListener("keydown", (e) => {
  if (e.isComposing || e.metaKey || e.ctrlKey || e.altKey) return;
  if (e.key === "Escape") {
    e.preventDefault();
    e.stopPropagation();
    $("#label-filter").open = false;
    $("#label-filter-summary").focus();
    return;
  }
  if (!$("#label-filter").open) return;
  const step = { ArrowDown: 1, j: 1, ArrowUp: -1, k: -1 }[
    e.key.length === 1 ? e.key.toLowerCase() : e.key
  ];
  if (!step) return;
  e.preventDefault();
  e.stopPropagation();
  const inputs = [...$("#label-options").querySelectorAll("input")];
  if (!inputs.length) return;
  const index = inputs.indexOf(e.target);
  const next = index < 0
    ? (step > 0 ? 0 : inputs.length - 1)
    : Math.max(0, Math.min(inputs.length - 1, index + step));
  inputs[next].focus({ preventScroll: true });
  inputs[next].closest(".label-option").scrollIntoView({ block: "nearest" });
});
$("#query").addEventListener("input", (e) => {
  state.filters.query = e.target.value;
  applyFilters();
});
$("#theme").addEventListener("click", () => {
  const next =
    document.documentElement.dataset.theme === "mocha" ? "latte" : "mocha";
  document.documentElement.dataset.theme = next;
  try {
    localStorage.setItem("mr-graph-theme", next);
  } catch {}
  updateThemeLabel();
});
function updateThemeLabel() {
  const label =
    document.documentElement.dataset.theme === "mocha"
      ? "ライトモードに切り替え"
      : "ダークモードに切り替え";
  $("#theme").title = `${label} (T)`;
  $("#theme").setAttribute("aria-label", label);
}
updateThemeLabel();
$("#refresh").addEventListener("click", refresh);
$("#fit").addEventListener("click", () => fit());
$("#zoom-in").addEventListener("click", () =>
  zoom(1.25, undefined, undefined, true),
);
$("#zoom-out").addEventListener("click", () =>
  zoom(0.8, undefined, undefined, true),
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
let drag = null,
  suppressClick = false;
canvas.addEventListener("pointerdown", (e) => {
  if (e.button !== 0 || e.target.closest("a, .zoom-tools")) return;
  drag = {
    x: e.clientX,
    y: e.clientY,
    startX: state.x,
    startY: state.y,
    pointerId: e.pointerId,
    moved: false,
  };
});
canvas.addEventListener("pointermove", (e) => {
  if (!drag || e.pointerId !== drag.pointerId) return;
  if (!drag.moved && Math.hypot(e.clientX - drag.x, e.clientY - drag.y) < 5)
    return;
  drag.moved = true;
  canvas.setPointerCapture(e.pointerId);
  canvas.classList.add("dragging");
  $("#world").classList.remove("animating");
  state.x = drag.startX + e.clientX - drag.x;
  state.y = drag.startY + e.clientY - drag.y;
  transform();
});
const stopDrag = (e) => {
  if (!drag || e.pointerId !== drag.pointerId) return;
  if (drag.moved) {
    suppressClick = true;
    setTimeout(() => {
      suppressClick = false;
    }, 0);
  }
  if (canvas.hasPointerCapture(e.pointerId))
    canvas.releasePointerCapture(e.pointerId);
  drag = null;
  canvas.classList.remove("dragging");
};
canvas.addEventListener("pointerup", stopDrag);
canvas.addEventListener("pointercancel", stopDrag);
window.addEventListener("pointerup", stopDrag);
window.addEventListener("pointercancel", stopDrag);
canvas.addEventListener("lostpointercapture", () => {
  drag = null;
  canvas.classList.remove("dragging");
});
canvas.addEventListener("click", (e) => {
  if (suppressClick) {
    e.preventDefault();
    e.stopPropagation();
    return;
  }
  if (e.target.closest("a, .zoom-tools")) return;
  const node = e.target.closest(".node-select");
  if (node) select(node.closest(".node").dataset.id);
  else clear();
});
canvas.addEventListener(
  "wheel",
  (e) => {
    if (e.target.closest(".zoom-tools")) return;
    e.preventDefault();
    $("#world").classList.remove("animating");
    const r = canvas.getBoundingClientRect();
    zoom(Math.exp(-e.deltaY * 0.0015), e.clientX - r.left, e.clientY - r.top);
  },
  { passive: false },
);
canvas.addEventListener("keydown", (e) => {
  if (
    e.target !== canvas ||
    state.selected ||
    e.metaKey ||
    e.ctrlKey ||
    e.altKey ||
    e.isComposing
  )
    return;
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
});
const shortcutControls = {
  "?": "help",
  "/": "query",
  a: "authored",
  s: "assigned",
  w: "approval-waiting",
  g: "label-filter-summary",
  b: "hide-bots",
  u: "auto",
  r: "refresh",
  t: "theme",
  f: "fit",
  0: "fit",
  "+": "zoom-in",
  "=": "zoom-in",
  "-": "zoom-out",
};
document.addEventListener("keydown", (e) => {
  if (e.isComposing || e.metaKey || e.ctrlKey || e.altKey) return;
  if ($("#shortcut-help").open) {
    if (e.key === "Escape") {
      e.preventDefault();
      $("#shortcut-help").close();
    }
    return;
  }
  const editable = e.target.closest(
    'input:not([type="checkbox"]), textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"]',
  );
  if (e.key === "Escape") {
    e.preventDefault();
    clear(!editable);
    if (editable) canvas.focus({ preventScroll: true });
    return;
  }
  if (editable) return;
  const direction = {
    h: "left",
    j: "down",
    k: "up",
    l: "right",
    ArrowLeft: "left",
    ArrowDown: "down",
    ArrowUp: "up",
    ArrowRight: "right",
  }[e.key.length === 1 ? e.key.toLowerCase() : e.key];
  if (state.selected && direction) {
    e.preventDefault();
    const next = neighboringMR(
      state.snapshot.graph,
      state.selected,
      direction,
      state.visible,
      state.positions,
    );
    if (next) {
      select(next);
      nodeElements
        .get(next)
        ?.querySelector(".node-select")
        ?.focus({ preventScroll: true });
    }
    return;
  }
  if (e.repeat) return;
  const id = shortcutControls[e.key.toLowerCase()];
  if (!id) return;
  const control = document.getElementById(id);
  if (control.disabled) return;
  e.preventDefault();
  if (id === "query") control.focus({ preventScroll: true });
  else {
    control.click();
    if (id === "label-filter-summary") {
      const target = $("#label-filter").open
        ? $("#label-options input") ?? control
        : control;
      target.focus({ preventScroll: true });
    }
  }
});
$("#help").addEventListener("click", () => $("#shortcut-help").showModal());
$("#close-help").addEventListener("click", () => $("#shortcut-help").close());
$("#shortcut-help").addEventListener("click", (e) => {
  if (e.target !== e.currentTarget) return;
  const r = e.currentTarget.getBoundingClientRect();
  if (
    e.clientX < r.left ||
    e.clientX > r.right ||
    e.clientY < r.top ||
    e.clientY > r.bottom
  )
    e.currentTarget.close();
});
new ResizeObserver(() => {
  if (state.fitted) fit(false);
}).observe(canvas);
poll();
