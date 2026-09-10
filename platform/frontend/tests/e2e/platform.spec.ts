import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("provider-driven create flow exposes mods only when supported", async ({ page }) => {
  await page.goto("/w/ember/instances/new");
  const modStep = page.getByRole("listitem").filter({ hasText: "模组" });
  await expect(modStep).toHaveCount(0);
  await page.getByLabel("游戏与版本").selectOption("prv_tmod_202506");
  await expect(modStep).toBeVisible();
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.getByText("公网地址与端口由系统部署时自动分配，协议由游戏 Provider 声明。")).toBeVisible();
});

test("instance lifecycle, logs, backup and restore are clickable", async ({ page }) => {
  await page.goto("/w/ember/instances/lin_terraria01");
  await expect(page.getByText("play.east.example:31777", { exact: true })).toHaveCount(1);
  await expect(page.getByText("TCP/UDP · 固定", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "停止" }).click();
  await expect(page.getByText("已停止", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "实时日志" }).click();
  await expect(page.getByText(/Logical instance lin_terraria01 attached/)).toBeVisible();
  await page.getByRole("button", { name: "备份" }).click();
  await page.getByRole("button", { name: "创建备份" }).click();
  await expect(page.getByText("手动备份", { exact: true })).toBeVisible();
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations.filter((item) => item.impact === "critical" || item.impact === "serious")).toEqual([]);
});

test("endpoint truth distinguishes stable, IP-only, and changeable bindings", async ({ page }) => {
  await page.goto("/w/ember/instances");
  await expect(page.getByText("203.0.113.18", { exact: true })).toBeVisible();
  await expect(page.getByText("UDP · 固定", { exact: true })).toBeVisible();
  await expect(page.getByText("TCP · 可能变化", { exact: true })).toBeVisible();
});
