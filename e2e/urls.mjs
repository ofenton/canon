// A reporting tool whose findings cannot be sent to somebody is much less useful than
// one whose can, so every view here has to survive being copied out of the address bar.
//
// The strong form of that is tested: a URL is reproduced in a *fresh page*, not merely
// asserted to have changed. A URL that updates but does not restore is worse than none,
// because it looks shareable.
//
//   node e2e/urls.mjs http://localhost:8080
import { chromium } from "playwright";

const base = process.argv[2] ?? "http://localhost:8080";
const failures = [];

function check(name, ok, detail) {
  console.log(`${ok ? "  PASS" : "  FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) failures.push(name);
}

const browser = await chromium.launch();
const errors = [];
const page = await browser.newPage();
page.on("pageerror", (e) => errors.push(String(e)));

const ready = (p) => p.waitForSelector("#main table, #main .blank, #main h2", { timeout: 10000 });
const view = (p) => p.locator('nav button[aria-current="true"]').getAttribute("data-view");

await page.goto(base);
await ready(page);

// AC: WHEN a view is reached THE SYSTEM SHALL update the URL so that opening it
// reproduces the view.
//
// Reached by keyboard and by pointer both, because the URL is written by navigate()
// and either path that skipped it would be a silent hole.
await page.keyboard.press("g");
await page.keyboard.press("f");
await page.waitForFunction(() => location.search.includes("view=flow"), { timeout: 8000 });
check("navigating updates the URL", true, await page.evaluate(() => location.search));

// Wait for the control, not for the URL: syncURL runs before render, so the address
// bar is correct a paint before the toolbar exists. Waiting on the URL and then
// asking whether #q is present is how this check first passed by skipping itself.
await page.click('nav button[data-view="work"]');
await page.waitForSelector("#q", { timeout: 8000 });
await page.locator("#q").selectOption("done");
await page.waitForFunction(() => location.search.includes("status=done"), { timeout: 8000 });
await page.waitForSelector("#next", { timeout: 8000 });
await page.locator("#next").click();
await page.waitForFunction(() => location.search.includes("offset="), { timeout: 8000 });

const url = page.url();
check("filters and the page are in the URL",
  url.includes("status=done") && url.includes("offset="), new URL(url).search);

// The reproduction: a page that has never seen the first one.
const fresh = await browser.newPage();
fresh.on("pageerror", (e) => errors.push(String(e)));
await fresh.goto(url);
await ready(fresh);
check("a copied URL reproduces the view", (await view(fresh)) === "work", await view(fresh));
check("a copied URL reproduces the filter", (await fresh.locator("#q").inputValue()) === "done");
const states = await fresh.locator("#main tbody .state").allTextContents();
check("the reproduced view shows the filtered rows",
  states.length > 0 && states.every((s) => s.trim() === "done"), `${states.length} row(s)`);
await fresh.close();

// A product detail is the view most worth sending to somebody, so it is reproduced too.
await page.click('nav button[data-view="products"]');
await page.waitForSelector("#main tbody tr", { timeout: 8000 });
await page.dblclick("#main tbody tr");
await page.waitForSelector("#main h2", { timeout: 8000 });
const detail = page.url();
check("a product has its own URL", detail.includes("product="), new URL(detail).search);

const deep = await browser.newPage();
deep.on("pageerror", (e) => errors.push(String(e)));
await deep.goto(detail);
await deep.waitForSelector("#main h2", { timeout: 10000 });
check("a product URL opens the product directly",
  (await deep.locator("#main h2").textContent()) === (await page.locator("#main h2").textContent()));
await deep.close();

// AC: WHEN the browser back button is used THE SYSTEM SHALL return to the previous view.
//
// Three views, then back twice and forward once — enough to catch a history that only
// ever replaces, which passes a single back and fails a sequence.
await page.goto(base);
await ready(page);
await page.click('nav button[data-view="work"]');
await page.waitForFunction(() => location.search.includes("view=work"), { timeout: 8000 });
await page.click('nav button[data-view="flow"]');
await page.waitForFunction(() => location.search.includes("view=flow"), { timeout: 8000 });
await page.click('nav button[data-view="conformance"]');
await page.waitForFunction(() => location.search.includes("view=conformance"), { timeout: 8000 });

await page.goBack();
await page.waitForFunction(() => location.search.includes("view=flow"), { timeout: 8000 });
check("back returns to the previous view", (await view(page)) === "flow");
await page.goBack();
await page.waitForFunction(() => location.search.includes("view=work"), { timeout: 8000 });
check("back again returns to the one before", (await view(page)) === "work");
await page.goForward();
await page.waitForFunction(() => location.search.includes("view=flow"), { timeout: 8000 });
check("forward moves back down the history", (await view(page)) === "flow");

// Escape out of a product uses the same history, so the address bar and the screen
// cannot disagree about where you are.
await page.click('nav button[data-view="products"]');
await page.waitForSelector("#main tbody tr", { timeout: 8000 });
await page.dblclick("#main tbody tr");
await page.waitForSelector("#main h2", { timeout: 8000 });
await page.keyboard.press("Escape");
await page.waitForFunction(() => !location.search.includes("product="), { timeout: 8000 });
check("leaving a product leaves its URL too", (await page.locator("#main h2").count()) === 0);

// AC: WHEN a URL naming a product that does not exist is opened THE SYSTEM SHALL say so
// rather than showing an empty screen.
//
// This is the failure mode a linkable UI creates: links outlive the thing they name.
await page.goto(base + "/?product=no-such-product");
await page.waitForSelector("#main .blank", { timeout: 10000 });
const said = await page.locator("#main .blank").textContent();
check("an unknown product says so", said.includes("no-such-product"), said.trim().split("\n")[0]);
check("and offers a way out", (await page.locator("#to-products").count()) === 1);
await page.click("#to-products");
await page.waitForSelector("#main tbody tr", { timeout: 8000 });
check("the way out works", (await view(page)) === "products");

check("no uncaught exceptions", errors.length === 0, errors[0]);

await browser.close();
console.log(failures.length ? `\n  ${failures.length} failure(s)` : "\n  all checks passed");
process.exit(failures.length ? 1 : 0);
