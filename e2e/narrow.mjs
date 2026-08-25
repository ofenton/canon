// The page at 400px, and every action from a pointer.
//
//   node e2e/narrow.mjs http://localhost:8080
import { chromium } from "playwright";

const base = process.argv[2] ?? "http://localhost:8080";
const failures = [];
function check(name, ok, detail) {
  console.log(`${ok ? "  PASS" : "  FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) failures.push(name);
}

const browser = await chromium.launch();
const errors = [];

// --- every action, by pointer only ---------------------------------------------
// Driven from the page's own registry rather than a list kept here, so an action added
// without a pointer path fails this without anybody updating the test.
const page = await browser.newPage();
page.on("pageerror", (e) => errors.push(String(e)));
await page.goto(base);
await page.waitForSelector("#main table", { timeout: 10000 });

const actions = await page.evaluate(() => ACTIONS.map((a) => ({ key: a.key, pointer: a.pointer })));
check("every action declares a pointer path", actions.every((a) => a.pointer),
  actions.filter((a) => !a.pointer).map((a) => a.key).join(", ") || `${actions.length} actions`);

for (const a of actions.filter((a) => a.pointer.startsWith("#") || a.pointer.startsWith("nav"))) {
  const n = await page.locator(a.pointer).count();
  check(`${a.key} has a control`, n === 1, a.pointer);
}

// And they do the thing, not merely exist.
await page.click("#help-btn");
check("the help control opens help", await page.locator("#help").isVisible());
await page.keyboard.press("Escape");

await page.click('nav [data-view="work"]');
await page.waitForFunction(() => location.search.includes("view=work"), { timeout: 8000 });
check("a nav control navigates", true);

// j and k declare "row": a pointer moves the selection by clicking the row it wants.
await page.waitForSelector("#main tbody tr", { timeout: 8000 });
const rows = await page.locator("#main tbody tr").count();
if (rows > 1) {
  const before = await page.locator('#main tbody tr[aria-selected="true"] .id').first().textContent();
  // The first cell, not the row's centre: the centre of a work row is the title, and
  // the title now opens on one click. The earlier version of this check clicked it,
  // navigated to a detail view, and compared the first row of a different table.
  await page.locator("#main tbody tr").nth(1).locator("td").first().click();
  const after = await page.locator('#main tbody tr[aria-selected="true"] .id').first().textContent();
  check("clicking a row moves the selection", before !== after, `${before?.trim()} → ${after?.trim()}`);
} else {
  check("clicking a row moves the selection", false, "needs more than one row");
}

await page.click('nav [data-view="products"]');
await page.waitForSelector("#main tbody tr", { timeout: 8000 });
await page.click("#main tbody .title");
await page.waitForSelector("#main h2", { timeout: 8000 });
check("one click on the title opens", await page.locator("#main h2").isVisible());

check("the back control appears in a detail view",
  !(await page.locator("#back-btn").isHidden()));
await page.click("#back-btn");
await page.waitForSelector("#main table", { timeout: 8000 });
check("the back control returns to the list", (await page.locator("#main h2").count()) === 0);
check("and hides again", await page.locator("#back-btn").isHidden());

await page.click("#reload-btn");
await page.waitForSelector("#main tbody tr", { timeout: 8000 });
check("the reload control reloads", (await page.locator("#status").textContent()).includes("reload"));
await page.close();

// --- 400px ----------------------------------------------------------------------
const narrow = await browser.newPage({ viewport: { width: 400, height: 800 } });
narrow.on("pageerror", (e) => errors.push(String(e)));

for (const view of ["products", "work", "flow", "conformance"]) {
  await narrow.goto(`${base}/?view=${view}`);
  // Every view paints something different — a table, a blank state, a set of figures —
  // so this waits for the view to have painted at all rather than for one shape.
  await narrow.waitForFunction(() => document.querySelector("#main").children.length > 0,
    { timeout: 10000 });
  const over = await narrow.evaluate(() => document.body.scrollWidth - document.documentElement.clientWidth);
  check(`${view} fits 400px`, over <= 0, `${over}px over`);
}

// Every column's content is shown, not hidden behind a sideways scroll.
await narrow.goto(`${base}/?view=work`);
await narrow.waitForSelector("#main tbody tr", { timeout: 10000 });
const labelled = await narrow.evaluate(() => {
  const row = document.querySelector("#main tbody tr");
  const cells = [...row.children];
  return {
    cells: cells.length,
    labelled: cells.filter((c) => c.dataset.label).length,
    stacked: getComputedStyle(cells[0]).display !== "table-cell",
  };
});
check("cells carry their column name", labelled.labelled === labelled.cells,
  `${labelled.labelled}/${labelled.cells}`);
check("the row stacks rather than scrolling", labelled.stacked);

// --- search --------------------------------------------------------------------
// One input, across every product, and in the URL — the state guard in internal/ui
// refuses a query that is not, which is why ui-001 came first.
const find = await browser.newPage();
find.on("pageerror", (e) => errors.push(String(e)));
await find.goto(base + "/?view=work");
await find.waitForSelector("#search", { timeout: 10000 });

const before = Number((await find.locator("#count").textContent()).match(/of (\d+)/)?.[1] ?? 0);
await find.locator("#search").type("revert", { delay: 20 });
await find.waitForFunction(() => location.search.includes("q=revert"), { timeout: 8000 });
const after = Number((await find.locator("#count").textContent()).match(/of (\d+)/)?.[1] ?? 0);
check("searching narrows the list", after > 0 && after < before, `${before} → ${after}`);
check("the query is in the URL", find.url().includes("q=revert"));

// Typing must not take the caret out of the box: the toolbar is rebuilt on every render,
// so the caret is briefly gone by construction and has to come back. Waiting rather than
// sampling — an immediate check reads the moment mid-render and fails a working build.
const keptFocus = await find.waitForFunction(
  () => document.activeElement?.id === "search", null, { timeout: 4000 })
  .then(() => true).catch(() => false);
check("the search box gets the caret back after the rebuild", keptFocus);

// Refining from a later page must not land on an empty one.
await find.goto(base + "/?view=work&offset=50");
await find.waitForSelector("#search", { timeout: 8000 });
await find.locator("#search").type("revert", { delay: 20 });
await find.waitForFunction(() => location.search.includes("q=revert"), { timeout: 8000 });
check("refining returns to the first page", !find.url().includes("offset="), new URL(find.url()).search);
check("and shows results rather than nothing",
  (await find.locator("#main tbody tr").count()) > 0);

const shouted = await browser.newPage();
await shouted.goto(base + "/?view=work&q=REVERT");
await shouted.waitForSelector("#main tbody tr, #main .blank", { timeout: 10000 });
check("case does not matter", (await shouted.locator("#main tbody tr").count()) > 0);
await shouted.close();

await find.goto(base + "/?view=work&q=zzzz-no-such-thing");
await find.waitForSelector("#main .blank", { timeout: 8000 });
check("a search with no results says what was searched",
  (await find.locator("#main .blank").textContent()).includes("every product"));
await find.close();

check("no uncaught exceptions", errors.length === 0, errors[0]);
await browser.close();
console.log(failures.length ? `\n  ${failures.length} failure(s)` : "\n  all checks passed");
process.exit(failures.length ? 1 : 0);
