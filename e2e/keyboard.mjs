// Canon's UI must be usable by keyboard alone and by pointer alone. Both paths
// dispatch through one action registry, and this drives each of them separately: a
// mouse-only run never sends a key, and a keyboard-only run never sends a click.
//
//   node e2e/keyboard.mjs http://localhost:8080
import { chromium } from "playwright";

const base = process.argv[2] ?? "http://localhost:8080";
const failures = [];

function check(name, ok, detail) {
  const line = ok ? "  PASS" : "  FAIL";
  console.log(`${line}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) failures.push(name);
}

const browser = await chromium.launch();
const consoleErrors = [];

// --- keyboard only -------------------------------------------------------------
const page = await browser.newPage();
page.on("pageerror", (e) => consoleErrors.push(String(e)));
await page.goto(base);
await page.waitForSelector("#main table, #main .blank", { timeout: 10000 });

await page.keyboard.press("?");
check("? opens keyboard help", await page.locator("#help").isVisible());
const helpRows = await page.locator("#help tbody tr").count();
check("help lists every action", helpRows >= 8, `${helpRows} rows`);
await page.keyboard.press("Escape");

const products = await page.locator("#main tbody tr").count();
check("products are listed", products >= 1, `${products} product(s)`);

check("a row is selected to begin with",
  (await page.locator('#main tbody tr[aria-selected="true"]').count()) === 1);

// Open the first product, which is the one that reads cleanly. A product Canon could
// not read shows its error instead of a table, which is correct and not what this
// check is about.
await page.keyboard.press("Enter");
await page.waitForSelector("#main h2", { timeout: 5000 });
check("Enter opens the product", await page.locator("#main h2").isVisible());
const increments = await page.locator("#main tbody tr").count();
check("the product shows its increments", increments >= 1, `${increments} increment(s)`);
const hint = await page.locator("#main .hint").last().textContent().catch(() => "");
check("changes are attributed to the ledger's history", hint.includes("commit"), hint.slice(0, 50));

await page.keyboard.press("Escape");
await page.waitForSelector("#main table", { timeout: 5000 });
check("Escape returns to the list", (await page.locator("#main h2").count()) === 0);

for (const [keys, marker] of [[["g", "w"], "Status"], [["g", "f"], "Cycle time"], [["g", "c"], "never enforced"]]) {
  for (const k of keys) await page.keyboard.press(k);
  await page.waitForFunction(
    (m) => document.querySelector("#main").textContent.includes(m), marker, { timeout: 8000 });
  check(`${keys.join(" ")} navigates`, true, marker);
}

// Movement needs more than one row to prove anything. The products list may hold
// exactly one — CI's fixture is a single checkout — so this is checked on work,
// which spans every increment of every product.
await page.keyboard.press("g");
await page.keyboard.press("w");
await page.waitForSelector("#main tbody tr", { timeout: 8000 });
const rows = await page.locator("#main tbody tr").count();
if (rows > 1) {
  await page.keyboard.press("j");
  const moved = await page.locator('#main tbody tr[aria-selected="true"] .id').first().textContent();
  await page.keyboard.press("k");
  const back = await page.locator('#main tbody tr[aria-selected="true"] .id').first().textContent();
  check("j and k move the selection", moved !== back, `${back.trim()} ↔ ${moved.trim()}`);
} else {
  check("j and k move the selection", false, "needs more than one row to test");
}

check("every screen says when it was read",
  (await page.locator("#derived").textContent()).length > 0);
check("no uncaught exceptions (keyboard)", consoleErrors.length === 0, consoleErrors[0]);
await page.close();

// --- pointer only --------------------------------------------------------------
const mouse = await browser.newPage();
const mouseErrors = [];
mouse.on("pageerror", (e) => mouseErrors.push(String(e)));
await mouse.goto(base);
await mouse.waitForSelector("#main table, #main .blank", { timeout: 10000 });

await mouse.click('nav button[data-view="work"]');
await mouse.waitForFunction(() => document.querySelector("#main").textContent.includes("Status"),
  { timeout: 8000 });
check("clicking a nav item navigates", true);

const filter = mouse.locator("#q");
if (await filter.count()) {
  await filter.selectOption("done");
  await mouse.waitForTimeout(600);
  const statuses = await mouse.locator("#main tbody .state").allTextContents();
  check("the status filter narrows the list",
    statuses.length > 0 && statuses.every((s) => s.trim() === "done"),
    `${statuses.length} row(s)`);
}

const next = mouse.locator("#next");
check("pagination controls are present", await next.count() === 1);

await mouse.click('nav button[data-view="products"]');
await mouse.waitForSelector("#main tbody tr", { timeout: 8000 });
await mouse.click("#main tbody tr");
check("clicking a row selects it",
  (await mouse.locator('#main tbody tr[aria-selected="true"]').count()) === 1);
await mouse.dblclick("#main tbody tr");
await mouse.waitForSelector("#main h2", { timeout: 5000 });
check("double-clicking opens the product", await mouse.locator("#main h2").isVisible());
check("no uncaught exceptions (mouse)", mouseErrors.length === 0, mouseErrors[0]);

await browser.close();
console.log(failures.length ? `\n  ${failures.length} failure(s)` : "\n  all checks passed");
process.exit(failures.length ? 1 : 0);
