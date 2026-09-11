import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const instanceListFixture = [
  { id: "lin_running", workspaceId: "ws_ember", name: "builder-creative-04", desiredState: "running", observedState: "running", game: { providerReleaseId: "gpr_terraria", key: "terraria", displayName: "Terraria", version: "1.4.5.8" }, endpoints: [{ name: "game", purpose: "join", address: "203.0.113.10", port: 32000, transports: ["tcp"], stability: "stable", displayAddress: "203.0.113.10:32000", primary: true }, { name: "query", purpose: "status", address: "203.0.113.10", port: 32001, transports: ["udp"], stability: "stable", displayAddress: "203.0.113.10:32001", primary: false }], resourceSpec: { cpuMilli: 1000, memoryMiB: 1024, diskGiB: 10 }, region: { id: "reg_asia_east", code: "asia-east", displayName: "亚洲东部" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
  { id: "lin_failed", workspaceId: "ws_ember", name: "a-very-long-terraria-instance-name-used-for-layout-verification", desiredState: "running", observedState: "failed", game: { providerReleaseId: "gpr_terraria", key: "terraria", displayName: "Terraria", version: "1.4.5.8" }, endpoints: [], resourceSpec: { cpuMilli: 1500, memoryMiB: 3072, diskGiB: 20 }, region: { id: "reg_asia_east", code: "asia-east", displayName: "亚洲东部" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
  { id: "lin_pending", workspaceId: "ws_ember", name: "calamity-infernum-03", desiredState: "running", observedState: "pending", game: { providerReleaseId: "gpr_tmod", key: "tmodloader", displayName: "tModLoader", version: "v2026.07.3.0" }, endpoints: [], resourceSpec: { cpuMilli: 4000, memoryMiB: 4096, diskGiB: 35 }, region: { id: "reg_asia_east", code: "asia-east", displayName: "亚洲东部" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
  { id: "lin_stopped", workspaceId: "ws_ember", name: "terraria-hardcore-01", desiredState: "stopped", observedState: "stopped", game: { providerReleaseId: "gpr_terraria", key: "terraria", displayName: "Terraria", version: "1.4.5.8" }, endpoints: [{ name: "game", purpose: "join", address: "203.0.113.18", transports: ["udp"], stability: "stable", displayAddress: "203.0.113.18", primary: true }], resourceSpec: { cpuMilli: 2000, memoryMiB: 2048, diskGiB: 20 }, region: { id: "reg_asia_east", code: "asia-east", displayName: "亚洲东部" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
];

test("instance list renders truthful states and supports whole-row keyboard navigation", async ({ page }) => {
  await page.route("**/control-plane/v1/workspaces", (route) => route.fulfill({ json: [{ id: "ws_ember", slug: "ember", name: "Ember Realms" }] }));
  let releaseInstances = () => {};
  const instanceGate = new Promise<void>((resolve) => { releaseInstances = resolve; });
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/instances$/, async (route) => {
    await instanceGate;
    await route.fulfill({ json: instanceListFixture });
  });

  await page.goto("/w/ember/instances");
  await expect(page.getByRole("region", { name: "实例列表" })).toHaveAttribute("aria-busy", "true");
  releaseInstances();
  await expect(page.getByText("运行中", { exact: true })).toBeVisible();
  await expect(page.getByText("等待部署", { exact: true })).toBeVisible();
  await expect(page.getByText("失败", { exact: true })).toBeVisible();
  await expect(page.getByText("已停止", { exact: true })).toBeVisible();
  await expect(page.getByText("203.0.113.10:32000", { exact: true })).toBeVisible();
  await expect(page.getByText("+1", { exact: true })).toBeVisible();
  await expect(page.getByText("未分配", { exact: true })).toBeVisible();
  await expect(page.getByText("亚洲东部", { exact: true }).first()).toBeVisible();

  const instanceLink = page.getByRole("link", { name: "打开实例 builder-creative-04" });
  await instanceLink.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/w\/ember\/instances\/lin_running$/);
});

test("instance list empty and narrow states preserve the primary task", async ({ page }) => {
  await page.route("**/control-plane/v1/workspaces", (route) => route.fulfill({ json: [{ id: "ws_ember", slug: "ember", name: "Ember Realms" }] }));
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/instances$/, (route) => route.fulfill({ json: [] }));
  await page.goto("/w/ember/instances");
  await expect(page.getByText("暂无实例", { exact: true })).toBeVisible();

  await page.unroute(/\/control-plane\/v1\/workspaces\/ws_ember\/instances$/);
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/instances$/, (route) => route.fulfill({ json: instanceListFixture }));
  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  await expect(page.getByText("builder-creative-04", { exact: true })).toBeVisible();
  await expect(page.getByText("203.0.113.10:32000", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations.filter((item) => item.impact === "critical" || item.impact === "serious")).toEqual([]);
});

test("provider-driven create flow exposes mods only when supported", async ({ page }) => {
  await page.goto("/w/ember/instances/new");
  const modStep = page.getByRole("listitem").filter({ hasText: "模组" });
  await expect(modStep).toHaveCount(0);
  await page.getByLabel("游戏与版本").selectOption("prv_tmod_202506");
  await expect(modStep).toBeVisible();
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.getByText("公网地址与端口由系统部署时自动分配，协议由游戏 Provider 声明。")).toBeVisible();
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.getByLabel("服务器名称")).toHaveValue("Ember Modded");
  await expect(page.getByLabel("附加启动参数")).toBeVisible();
  await page.getByRole("checkbox", { name: "启动时更新模组" }).uncheck();
  await expect(page.getByLabel("附加启动参数")).toHaveCount(0);
  await page.getByRole("checkbox", { name: "启动时更新模组" }).check();
  await page.getByRole("button", { name: "下一步" }).click();
  await page.getByRole("checkbox", { name: "Calamity Mod" }).check();
  await expect(page.getByLabel("Calamity Mod 版本")).toBeEnabled();
});

test("instance lifecycle, logs, backup and restore are clickable", async ({ page }) => {
  await page.goto("/w/ember/instances/lin_terraria01");
  await expect(page.getByText("play.east.example:31777", { exact: true })).toHaveCount(1);
  await expect(page.getByText("TCP/UDP · 固定", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "停止" }).click();
  await expect(page.getByText("已停止", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "终端控制台" }).click();
  await page.getByLabel("控制台命令").fill("status");
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page.getByText("Command accepted")).toBeVisible();
  await page.getByRole("button", { name: "实时日志" }).click();
  await expect(page.getByText(/Logical instance lin_terraria01 attached/)).toBeVisible();
  await page.getByRole("button", { name: "备份" }).click();
  await page.getByRole("button", { name: "创建备份" }).click();
  await expect(page.getByText("手动备份", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "恢复", exact: true }).first().click();
  await expect(page.getByRole("button", { name: "恢复中" }).first()).toBeDisabled();
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations.filter((item) => item.impact === "critical" || item.impact === "serious")).toEqual([]);
});

test("endpoint truth distinguishes stable, IP-only, and changeable bindings", async ({ page }) => {
  await page.goto("/w/ember/instances");
  await expect(page.getByText("203.0.113.18", { exact: true })).toBeVisible();
  await expect(page.getByText("UDP · 固定", { exact: true })).toBeVisible();
  await expect(page.getByText("TCP · 可能变化", { exact: true })).toBeVisible();
});
