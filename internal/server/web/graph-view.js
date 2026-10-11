"use strict";

function isBot(mr) {
  // GitLab commonly gives project bot accounts a suffix (renovate_bot, etc.).
  return /(?:^|[^a-z])(dependabot|renovate)(?:$|[^a-z])/i.test(
    `${mr.author} ${mr.authorUser?.name ?? ""}`,
  );
}
function matchesMR(mr, filters, user) {
  if (filters.hideBots && isBot(mr)) return false;
  const sameUser = (person) =>
    !!user &&
    ((user.id > 0 && person?.id === user.id) ||
      (!!user.username && person?.username === user.username));
  if (filters.authored || filters.assigned) {
    const authored =
      sameUser(mr.authorUser) ||
      (!!user?.username && mr.author === user.username);
    const assigned = (mr.assignees ?? []).some(sameUser);
    if (!(filters.authored && authored) && !(filters.assigned && assigned))
      return false;
  }
  const query = filters.query
    .trim()
    .toLocaleLowerCase()
    .replace(/^!(?=\d)/, "");
  return [
    mr.iid,
    mr.title,
    mr.author,
    mr.authorUser?.name,
    mr.sourceBranch,
    mr.targetBranch,
    ...(mr.assignees ?? []).flatMap((u) => [u.username, u.name]),
    ...(mr.labels ?? []).map((l) => l.name),
  ]
    .join(" ")
    .toLocaleLowerCase()
    .includes(query);
}
function graphVisibility(graph, filters) {
  const matches = new Set(
    graph.nodes
      .filter(
        (n) => n.mr && matchesMR(n.mr, filters, graph.project.currentUser),
      )
      .map((n) => n.id),
  );
  const visible = new Set(matches);
  const parents = new Map();
  for (const e of graph.edges) {
    if (!parents.has(e.target)) parents.set(e.target, []);
    parents.get(e.target).push(e.source);
  }
  const queue = [...matches];
  for (let i = 0; i < queue.length; i++) {
    for (const id of parents.get(queue[i]) ?? []) {
      if (!visible.has(id)) {
        visible.add(id);
        queue.push(id);
      }
    }
  }
  return { matches, visible };
}

// Build a compact spanning forest of the visible graph. All cross/cycle edges
// remain present; the visited set prevents cycles from recursing forever.
function layoutVisibleGraph(graph, visible) {
  const list = graph.nodes.filter((n) => visible.has(n.id));
  const children = new Map(),
    forest = new Map(),
    visited = new Set(),
    parent = new Set();
  for (const e of graph.edges) {
    if (!visible.has(e.source) || !visible.has(e.target)) continue;
    if (!children.has(e.source)) children.set(e.source, []);
    children.get(e.source).push(e.target);
  }
  for (const values of children.values()) values.sort();
  list.sort(
    (a, b) =>
      Number(!!a.mr) - Number(!!b.mr) ||
      (a.mr ? a.mr.iid - b.mr.iid : a.label.localeCompare(b.label)),
  );
  const visit = (id) => {
    visited.add(id);
    for (const next of children.get(id) ?? []) {
      if (visited.has(next)) continue;
      parent.add(next);
      if (!forest.has(id)) forest.set(id, []);
      forest.get(id).push(next);
      visit(next);
    }
  };
  for (const n of list) if (!visited.has(n.id)) visit(n.id);
  let row = 0;
  const positions = new Map();
  const place = (id, depth) => {
    const family = forest.get(id) ?? [];
    const ys = family.map((next) => place(next, depth + 1));
    const y = ys.length ? (ys[0] + ys[ys.length - 1]) / 2 : 36 + row++ * 224;
    positions.set(id, { x: 36 + depth * 344, y });
    return y;
  };
  for (const n of list)
    if (!parent.has(n.id)) {
      place(n.id, 0);
      row++;
    }
  return positions;
}

function neighboringMR(graph, selected, direction, visible, positions) {
  const current = positions.get(selected);
  if (!current) return null;
  const parents = new Set(
    graph.edges.filter((e) => e.target === selected).map((e) => e.source),
  );
  let candidates;
  if (direction === "left") candidates = parents;
  else if (direction === "right")
    candidates = new Set(
      graph.edges.filter((e) => e.source === selected).map((e) => e.target),
    );
  else
    candidates = new Set(
      graph.edges.filter((e) => parents.has(e.source)).map((e) => e.target),
    );
  return (
    graph.nodes
      .filter((n) => {
        const p = positions.get(n.id);
        return (
          n.mr &&
          n.id !== selected &&
          visible.has(n.id) &&
          candidates.has(n.id) &&
          p &&
          (direction !== "up" || p.y < current.y) &&
          (direction !== "down" || p.y > current.y)
        );
      })
      .sort((a, b) => {
        const pa = positions.get(a.id),
          pb = positions.get(b.id);
        return (
          Math.abs(pa.y - current.y) - Math.abs(pb.y - current.y) ||
          Math.abs(pa.x - current.x) - Math.abs(pb.x - current.x) ||
          a.mr.iid - b.mr.iid
        );
      })[0]?.id ?? null
  );
}

if (typeof module !== "undefined")
  module.exports = {
    isBot,
    matchesMR,
    graphVisibility,
    layoutVisibleGraph,
    neighboringMR,
  };
