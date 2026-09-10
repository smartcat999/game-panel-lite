import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";

function captureUnexpectedBrowserErrors(page: Page) {
  const errors: string[] = [];
  page.on("console", message => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", error => errors.push(error.message));
  return () => expect(errors).toEqual([]);
}

test("customer creates a versioned Terraria checkout", async ({ page }) => {
  const expectNoBrowserErrors = captureUnexpectedBrowserErrors(page);
  await page.goto("/w/northstar/instances/new");
  await page.getByLabel("Instance name").fill("Provider Contract World");
  await page.getByLabel("Plan").selectOption({ index: 1 });
  await page.getByLabel("Region").selectOption({ index: 1 });
  await expect(page.getByLabel("Game version")).toHaveValue("1.4.5.6");
  await page.getByRole("button", { name: "Continue to payment", exact: true }).click();
  await expect(page.getByText("Payment is required before this instance can be deployed.", { exact: true })).toBeVisible();
  await expect(page.getByText("1.4.5.6", { exact: true })).toBeVisible();
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations.filter(item => item.impact === "critical" || item.impact === "serious")).toEqual([]);
  expectNoBrowserErrors();
});

test("backup request and same-Region restore remain accessible", async ({ page }) => {
  const expectNoBrowserErrors = captureUnexpectedBrowserErrors(page);
  await page.goto("/w/northstar/backups");
  await expect(page.getByText("bkr_000001", { exact: true })).toBeVisible();
  await page.locator("select").nth(2).selectOption("lin_000003");
  await page.getByRole("button", { name: "Create backup", exact: true }).click();
  await expect(page.getByRole("cell", { name: "Queued", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Restore", exact: true }).click();
  await expect(page.getByRole("cell", { name: "Restore", exact: true })).toBeVisible();
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations.filter(item => item.impact === "critical" || item.impact === "serious")).toEqual([]);
  expectNoBrowserErrors();
});

test("locale, dark theme, monitoring, and mobile navigation", async ({ page }) => {
  const expectNoBrowserErrors = captureUnexpectedBrowserErrors(page);
  await page.goto("/platform/regions/reg_asia_east/monitoring");
  await page.locator("select").nth(0).selectOption("zh-CN");
  await page.locator("select").nth(1).selectOption("dark");
  await expect(page.getByText("任务延迟（毫秒）", { exact: true })).toBeVisible();
  await expect(page.getByText("协调失败", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  await page.locator("select").nth(1).selectOption("light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.locator("select").nth(1).selectOption("dark");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByRole("button", { name: "打开导航", exact: true })).toBeVisible();
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations.filter(item => item.impact === "critical" || item.impact === "serious")).toEqual([]);
  expectNoBrowserErrors();
});
