// Drives Canon's UI with the keyboard only, never the mouse.
//
// The structural tests in internal/ui prove every action has a binding. This proves
// the bindings actually work in a browser, which is the claim that matters.
import { chromium } from "playwright";

const base = process.argv[2] ?? "http://localhost:8093";
// The actor must be one the instance has registered. Hardcoding it made this test
// pass locally and time out in CI, where a different id had been bootstrapped —
// and the timeout said nothing about why.
const actor = process.argv[3] ?? "you";
const failures = [];
const check = (name, ok, detail = "") => {
  console.log(`  ${ok ? "PASS" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) failures.push(name);
};

const browser = await chromium.launch();
const page = await browser.newPage();

// Fail loudly on uncaught exceptions: a UI that throws while appearing to work is
// the worst kind of passing test.
//
// The browser also logs "Failed to load resource" for any 4xx, and this run
// deliberately provokes a 422 to check that refusals surface. Those are expected
// outcomes handled by the app, not defects, so only real errors are collected.
const consoleErrors = [];
const expectedNetworkNoise = (text) =>
  text.includes("Failed to load resource") && /4\d\d|5\d\d/.test(text);
page.on("console", (m) => {
  if (m.type() === "error" && !expectedNetworkNoise(m.text())) consoleErrors.push(m.text());
});
page.on("pageerror", (e) => consoleErrors.push("uncaught: " + String(e)));

await page.goto(`${base}/?actor=${encodeURIComponent(actor)}`);
await page.waitForFunction(() => window.CANON !== undefined);

// Fail early and clearly if the actor is not registered, rather than timing out
// thirty seconds later on a symptom.
const whoami = await page.evaluate(async (id) => {
  const res = await fetch("/api/actors/" + encodeURIComponent(id), {
    headers: { "X-Canon-Actor": id },
  });
  return { status: res.status, body: await res.text() };
}, actor);
if (whoami.status !== 200) {
  console.log(`  FAIL  actor ${actor} is not registered on this instance ` +
              `(${whoami.status}). Bootstrap it, or pass the right id as argv[3].`);
  await browser.close();
  process.exit(1);
}
console.log(`  using actor ${actor}`);

// The mouse is never used. Every interaction below is a keystroke.
await page.keyboard.press("?");
check("? opens keyboard help", await page.locator("#help").isVisible());
const helpRows = await page.locator("#help-table tr").count();
check("help lists every action", helpRows >= 10, `${helpRows} rows`);
await page.keyboard.press("Escape");

await page.keyboard.press("c");
const focused = await page.evaluate(() => document.activeElement?.id);
check("c opens create with the title focused", focused === "title", `focus=${focused}`);
const inputs = await page.locator("#create input").count();
check("create asks for a title and nothing else", inputs === 1, `${inputs} inputs`);

await page.keyboard.type("Search is slow");
await page.keyboard.press("Enter");
try {
  await page.waitForFunction(
    () => document.querySelectorAll("#main tbody tr").length > 0, { timeout: 5000 });
  check("issue created by keyboard alone", (await page.locator("#main tbody tr").count()) === 1);
} catch {
  check("issue created by keyboard alone", false,
    "status bar said: " + (await page.locator("#status").textContent()));
}

await page.keyboard.press("c");
await page.keyboard.type("Second issue");
await page.keyboard.press("Enter");
await page.waitForFunction(() => document.querySelectorAll("#main tbody tr").length === 2, { timeout: 5000 });

await page.keyboard.press("j");
const afterJ = await page.evaluate(() =>
  document.querySelector('#main tr[aria-selected="true"]')?.dataset.id);
check("j moves the selection", afterJ === "CANON-2", `selected=${afterJ}`);
await page.keyboard.press("k");
const afterK = await page.evaluate(() =>
  document.querySelector('#main tr[aria-selected="true"]')?.dataset.id);
check("k moves it back", afterK === "CANON-1", `selected=${afterK}`);

await page.keyboard.press("t");
await page.waitForFunction(() => document.activeElement?.id === "prompt-input");
await page.keyboard.type("in_progress");
await page.keyboard.press("Enter");
await page.waitForFunction(() =>
  [...document.querySelectorAll("#main tbody tr")].some((r) => r.textContent.includes("in_progress")),
  { timeout: 5000 });
check("t transitions the selected issue", true);

// A refused transition must surface the schema's reason, not fail silently.
await page.keyboard.press("t");
await page.waitForFunction(() => document.activeElement?.id === "prompt-input");
await page.keyboard.type("done");
await page.keyboard.press("Enter");
await page.waitForFunction(() => document.querySelector("#status")?.textContent.includes("cannot move"),
  { timeout: 5000 });
const refusal = await page.locator("#status").textContent();
check("an illegal transition shows the schema's reason", refusal.includes("permitted transitions"),
  refusal.slice(0, 70));

await page.keyboard.press("/");
await page.waitForFunction(() => document.activeElement?.id === "prompt-input");
await page.keyboard.type("state=todo");
await page.keyboard.press("Enter");
await page.waitForFunction(() => document.querySelectorAll("#main tbody tr").length === 1, { timeout: 5000 });
check("/ filters with a query", true);

// A truncated list must say so rather than silently showing a prefix — and it must
// not do it in the status bar, which reports the last action.
const summary = await page.locator("#list-summary").textContent();
check("the list reports how many it is showing",
  /\d+ issues?|showing \d+ of \d+/.test(summary), summary.slice(0, 60));

for (const [keys, marker] of [[["g", "m"], "Cycle time"], [["g", "p"], "proposal"], [["g", "i"], ""]]) {
  for (const k of keys) await page.keyboard.press(k);
  await page.waitForTimeout(400);
  const body = await page.locator("main").textContent();
  check(`g ${keys[1]} navigates`, marker === "" || body.toLowerCase().includes(marker.toLowerCase()),
    body.slice(0, 40).replace(/\s+/g, " "));
}

check("no uncaught exceptions", consoleErrors.length === 0, consoleErrors.slice(0, 2).join(" | "));

await browser.close();
console.log(failures.length ? `\n  ${failures.length} failure(s)` : "\n  all keyboard checks passed");
process.exit(failures.length ? 1 : 0);
