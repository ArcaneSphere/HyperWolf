import assert from "node:assert/strict";
import { chromium } from "playwright-core";

const baseURL = process.env.HYPERWOLF_URL || "http://127.0.0.1:18080";
const executablePath = process.env.CHROME_BIN || "/usr/bin/google-chrome";
const interactive = process.argv.includes("--interactive");
const tela = process.argv.includes("--tela");
const errors = [];

const browser = await chromium.launch({
  headless: true,
  executablePath,
  args: ["--no-sandbox"],
});

try {
  const page = await browser.newPage();
  page.on("pageerror", (error) => errors.push(error.message));

  const response = await page.goto(`${baseURL}/`, { waitUntil: "networkidle" });
  assert.equal(response?.status(), 200, "dashboard should return HTTP 200");
  assert.equal(await page.title(), "HyperWolf");
  await assertElement(page, "#searchBox");
  await assertElement(page, "#connectNodeBtn");

  const config = await page.evaluate(async () => {
    const response = await fetch("/api/config");
    return response.json();
  });
  assert.equal(config.ok, true, "dashboard config should be available");
  assert.match(config.result.version, /^\d+\.\d+\.\d+$/);

  const aboutVersion = await page.locator("#hw-version").textContent();
  assert.equal(aboutVersion?.trim(), config.result.version);

  if (interactive || tela) {
    const searchBox = page.locator("#searchBox");
    await assertElement(page, "#searchBox:not([disabled])");
    await searchBox.fill("Derotary");
    await page.waitForTimeout(400);
    assert.equal(await page.locator("#results .result").count(), 0);
    assert.ok(
      await page.locator("#searchSuggestions:not(.hidden) .search-suggestion").count() > 0,
      "typing should show search suggestions",
    );

    await searchBox.press("Enter");
    await page.locator("#results .result").first().waitFor();
    assert.equal(await page.locator("#results .nameHdr").first().textContent(), "Derotary");

    if (tela) {
      const childPromise = page.context().waitForEvent("page", { timeout: 20_000 });
      await page.locator("#results .result").first().click();
      const child = await childPromise;
      await child.waitForLoadState("domcontentloaded", { timeout: 20_000 });
      const childURL = new URL(child.url());
      assert.ok(
        childURL.hostname === "localhost" || childURL.hostname === "127.0.0.1",
        `TELA app should load through loopback, got ${child.url()}`,
      );
      assert.equal(await child.title(), "Derotary");
    }
  }

  assert.deepEqual(errors, [], `dashboard page errors: ${errors.join("; ")}`);
  const modes = [interactive ? "interactive search" : "", tela ? "TELA load" : ""].filter(Boolean);
  console.log(`browser smoke passed (${config.result.version}${modes.length ? `, ${modes.join(", ")}` : ""})`);
} finally {
  await browser.close();
}

async function assertElement(page, selector) {
  const count = await page.locator(selector).count();
  assert.equal(count, 1, `expected ${selector} to exist exactly once`);
}
