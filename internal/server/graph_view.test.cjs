const { test } = require("node:test");
const assert = require("node:assert/strict");
const {
  matchesMR,
  isBot,
  graphVisibility,
  layoutVisibleGraph,
  neighboringMR,
} = require("./web/graph-view.js");
const me = { id: 1, username: "me" };
const all = { authored: false, assigned: false, hideBots: false, query: "" };
const mr = (id, extra = {}) => ({
  iid: id,
  title: `Change ${id}`,
  author: "other",
  authorUser: { id: 2, username: "other" },
  ...extra,
});
test("navigation follows direct parents/children and siblings sharing a parent", () => {
  const g = {
    project: {},
    nodes: [
      { id: "main", label: "main" },
      ...[1, 2, 3, 4, 5, 6].map((id) => node(id)),
    ],
    edges: [
      { source: "main", target: "1" },
      { source: "1", target: "2" },
      { source: "1", target: "3" },
      { source: "2", target: "4" },
      { source: "main", target: "5" },
      { source: "5", target: "6" },
    ],
  };
  const positions = new Map([
    ["main", { x: 0, y: 100 }],
    ["1", { x: 100, y: 100 }],
    ["2", { x: 200, y: 0 }],
    ["3", { x: 200, y: 200 }],
    ["4", { x: 300, y: 0 }],
    ["5", { x: 100, y: 400 }],
    ["6", { x: 200, y: 400 }],
  ]);
  const visible = new Set(g.nodes.map((n) => n.id));
  const go = (id, direction, shown = visible) =>
    neighboringMR(g, id, direction, shown, positions);
  assert.equal(go("2", "left"), "1");
  assert.equal(go("2", "right"), "4");
  assert.equal(go("2", "down"), "3");
  assert.equal(go("3", "up"), "2");
  assert.equal(go("3", "down"), null); // 6 has a different parent.
  assert.equal(go("1", "left"), null); // Default branch is not an MR.
  assert.equal(go("4", "right"), null);
  assert.equal(go("2", "down", new Set(["1", "2", "4"])), null);
  assert.equal(go("1", "right"), "2"); // Equal-distance candidates use IID.
});
test("navigation considers shared parents in a DAG and handles cycles", () => {
  const g = {
    project: {},
    nodes: [1, 2, 3, 4].map((id) => node(id)),
    edges: [
      { source: "1", target: "3" },
      { source: "2", target: "3" },
      { source: "2", target: "4" },
      { source: "3", target: "1" },
    ],
  };
  const positions = new Map([
    ["1", { x: 0, y: 10 }],
    ["2", { x: 0, y: 90 }],
    ["3", { x: 100, y: 80 }],
    ["4", { x: 100, y: 180 }],
  ]);
  const visible = new Set(["1", "2", "3", "4"]);
  assert.equal(neighboringMR(g, "3", "left", visible, positions), "2");
  assert.equal(neighboringMR(g, "3", "down", visible, positions), "4");
  assert.equal(neighboringMR(g, "3", "right", visible, positions), "1");
});
const node = (id, extra = {}) => ({ id: String(id), mr: mr(id, extra) });
const graph = {
  project: { currentUser: me },
  nodes: [
    { id: "main", kind: "root", label: "main" },
    node(1),
    node(2, { title: "Find me" }),
    node(3),
    node(4),
  ],
  edges: [
    { source: "main", target: "1" },
    { source: "1", target: "2" },
    { source: "1", target: "3" },
    { source: "main", target: "4" },
  ],
};
test("Authored and Assigned combine with OR and bot/query filters with AND", () => {
  const authored = mr(1, { author: "me", authorUser: me });
  const assigned = mr(2, { assignees: [me] });
  assert.ok(matchesMR(authored, { ...all, authored: true }, me));
  assert.ok(matchesMR(assigned, { ...all, assigned: true }, me));
  assert.ok(
    matchesMR(assigned, { ...all, authored: true, assigned: true }, me),
  );
  assert.ok(
    matchesMR(authored, { ...all, authored: true, assigned: true }, me),
  );
  assert.equal(
    matchesMR(mr(3), { ...all, authored: true, assigned: true }, me),
    false,
  );
  assert.equal(
    matchesMR(authored, { ...all, authored: true, query: "no match" }, me),
    false,
  );
  assert.equal(matchesMR(authored, { ...all, authored: true }, null), false);
});
test("query covers users, branches, labels and MR numbers", () => {
  const m = mr(42, {
    assignees: [{ name: "Assigned Name" }],
    labels: [{ name: "Backend" }],
    sourceBranch: "feature/auth",
  });
  for (const query of ["!42", "assigned name", "BACKEND", "feature/auth"])
    assert.ok(matchesMR(m, { ...all, query }, me));
});
test("only Dependabot and Renovate accounts count as bots", () => {
  for (const author of [
    "dependabot",
    "renovate",
    "project_renovate_bot",
    "dependabot[bot]",
  ]) {
    assert.ok(isBot(mr(1, { author })));
    assert.ok(matchesMR(mr(1, { author }), all, me));
    assert.equal(
      matchesMR(mr(1, { author }), { ...all, hideBots: true }, me),
      false,
    );
  }
  for (const author of ["robot", "myrenovatedesign", "dependabotany"])
    assert.equal(isBot(mr(1, { author })), false);
});
test("numeric query also matches the downstream MR target branch", () => {
  const g = {
    project: { currentUser: me },
    nodes: [
      { id: "main", kind: "root", label: "main" },
      node(204, { sourceBranch: "change-204", targetBranch: "main" }),
      node(205, { sourceBranch: "change-205", targetBranch: "change-204" }),
      node(206, { sourceBranch: "change-206", targetBranch: "change-205" }),
    ],
    edges: [
      { source: "main", target: "204" },
      { source: "204", target: "205" },
      { source: "205", target: "206" },
    ],
  };
  const { matches, visible } = graphVisibility(g, { ...all, query: "205" });
  assert.deepEqual([...matches], ["205", "206"]);
  assert.deepEqual([...visible].sort(), ["204", "205", "206", "main"]);
});
test("matching descendants retain every upstream path and hide unrelated siblings", () => {
  const { matches, visible } = graphVisibility(graph, {
    ...all,
    query: "Find me",
  });
  assert.deepEqual([...matches], ["2"]);
  assert.deepEqual([...visible].sort(), ["1", "2", "main"]);
  const positions = layoutVisibleGraph(graph, visible);
  assert.equal(positions.size, 3);
  assert.equal(positions.get("1").y, positions.get("2").y);
  assert.ok(positions.get("2").x > positions.get("1").x);
  assert.equal(
    graphVisibility(graph, { ...all, query: "absent" }).visible.size,
    0,
  );
});
test("cycles and multiple parents retain all nodes without infinite traversal", () => {
  const g = {
    ...graph,
    edges: [
      ...graph.edges,
      { source: "2", target: "1" },
      { source: "4", target: "2" },
    ],
  };
  const { visible } = graphVisibility(g, { ...all, query: "Find me" });
  assert.deepEqual([...visible].sort(), ["1", "2", "4", "main"]);
  const positions = layoutVisibleGraph(g, visible);
  assert.equal(positions.size, 4);
  for (const p of positions.values())
    assert.ok(Number.isFinite(p.x) && Number.isFinite(p.y));
});

test("approval waiting includes drafts and excludes unknown or completed approval", () => {
  const filters = { ...all, approvalWaiting: true };
  assert.ok(matchesMR(mr(1, { draft: true, approval: { state: "waiting" } }), filters, me));
  assert.ok(matchesMR(mr(1, { draft: true, approval: { state: "unapproved" } }), filters, me));
  for (const state of ["met", "not_required", "received", "unknown", "loading"])
    assert.equal(matchesMR(mr(1, { approval: { state } }), filters, me), false);
  assert.equal(matchesMR(mr(1), filters, me), false);
});

test("selected labels require every exact label and combine with existing filters", () => {
  const m = mr(1, {
    author: "me", authorUser: me, approval: { state: "waiting" },
    labels: [{ name: "backend" }, { name: "urgent" }],
  });
  const filters = { ...all, approvalWaiting: true, labels: ["backend", "urgent"] };
  assert.ok(matchesMR(m, { ...filters, authored: true, query: "Change" }, me));
  assert.equal(matchesMR(m, { ...filters, labels: ["backend", "missing"] }, me), false);
  assert.equal(matchesMR(m, { ...filters, labels: ["Backend"] }, me), false);
  assert.equal(matchesMR(mr(2, { approval: { state: "waiting" } }), filters, me), false);
  assert.equal(matchesMR({ ...m, approval: { state: "met" } }, filters, me), false);
  assert.equal(matchesMR(m, { ...filters, assigned: true }, me), false);
  assert.equal(matchesMR(m, { ...filters, query: "missing" }, me), false);
  assert.equal(matchesMR({ ...m, author: "renovate", authorUser: {} }, { ...filters, hideBots: true }, me), false);
  assert.ok(matchesMR(mr(3), { ...all, labels: [] }, me));
});

test("approval and label filters preserve nonmatching ancestors", () => {
  const g = {
    project: { currentUser: me },
    nodes: [
      { id: "main", kind: "root", label: "main" },
      node(1, { approval: { state: "met" } }),
      node(2, { approval: { state: "waiting" }, labels: [{ name: "backend" }, { name: "urgent" }] }),
      node(3, { approval: { state: "waiting" }, labels: [{ name: "backend" }] }),
    ],
    edges: [{ source: "main", target: "1" }, { source: "1", target: "2" }, { source: "main", target: "3" }],
  };
  const { matches, visible } = graphVisibility(g, { ...all, approvalWaiting: true, labels: ["backend", "urgent"] });
  assert.deepEqual([...matches], ["2"]);
  assert.deepEqual([...visible].sort(), ["1", "2", "main"]);
});
