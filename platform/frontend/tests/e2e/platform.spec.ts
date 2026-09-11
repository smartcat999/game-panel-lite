import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const instanceListFixture = [
  { id: "lin_running", workspaceId: "ws_ember", name: "builder-creative-04", desiredState: "running", observedState: "running", game: { providerReleaseId: "gpr_terraria", key: "terraria", displayName: "Terraria", version: "1.4.5.8" }, endpoints: [{ name: "game", purpose: "join", address: "203.0.113.10", port: 32000, transports: ["tcp"], stability: "stable", displayAddress: "203.0.113.10:32000", primary: true }, { name: "query", purpose: "status", address: "203.0.113.10", port: 32001, transports: ["udp"], stability: "stable", displayAddress: "203.0.113.10:32001", primary: false }], resourceSpec: { cpuMilli: 1000, memoryMiB: 1024, diskGiB: 10 }, region: { id: "reg_asia_east", code: "asia-east", displayName: "亚洲东部" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
  { id: "lin_failed", workspaceId: "ws_ember", name: "a-very-long-terraria-instance-name-used-for-layout-verification", desiredState: "running", observedState: "failed", game: { providerReleaseId: "gpr_terraria", key: "terraria", displayName: "Terraria", version: "1.4.5.8" }, endpoints: [], resourceSpec: { cpuMilli: 1500, memoryMiB: 3072, diskGiB: 20 }, region: { id: "reg_asia_east", code: "asia-east", displayName: "亚洲东部" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
  { id: "lin_pending", workspaceId: "ws_ember", name: "calamity-infernum-03", desiredState: "running", observedState: "pending", game: { providerReleaseId: "gpr_tmod", key: "tmodloader", displayName: "tModLoader", version: "v2026.07.3.0" }, endpoints: [], resourceSpec: { cpuMilli: 4000, memoryMiB: 4096, diskGiB: 35 }, region: { id: "reg_asia_east", code: "asia-east", displayName: "亚洲东部" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
  { id: "lin_stopped", workspaceId: "ws_ember", name: "terraria-hardcore-01", desiredState: "stopped", observedState: "stopped", game: { providerReleaseId: "gpr_terraria", key: "terraria", displayName: "Terraria", version: "1.4.5.8" }, endpoints: [{ name: "game", purpose: "join", address: "203.0.113.18", transports: ["udp"], stability: "stable", displayAddress: "203.0.113.18", primary: true }], resourceSpec: { cpuMilli: 2000, memoryMiB: 2048, diskGiB: 20 }, region: { id: "reg_asia_east", code: "asia-east", displayName: "亚洲东部" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
];

const terrariaManifest = {
  providerReleaseId: "prv_terraria", gameKey: "terraria", displayName: "Terraria", releaseVersion: "2", gameVersions: ["1.4.5.8"], capabilities: ["configuration"], schemaVersion: 1,
  configurationSchema: { required: ["worldName", "worldSize"], properties: {
    worldName: { type: "string", title: "世界名称", localizations: { "zh-CN": { title: "世界名称", default: "泰拉瑞亚世界" }, en: { title: "World name", default: "GamePanel World" } }, applyBehavior: "create-only", default: "GamePanel World" },
    worldSize: { type: "enum", title: "世界大小", localizations: { "zh-CN": { title: "世界大小", enumLabels: { small: "小型", medium: "中型", large: "大型" } }, en: { title: "World size", enumLabels: { small: "Small", medium: "Medium", large: "Large" } } }, applyBehavior: "create-only", default: "medium", enum: ["small", "medium", "large"] },
    maxPlayers: { type: "integer", title: "最大玩家数", localizations: { "zh-CN": { title: "最大玩家数" }, en: { title: "Maximum players" } }, applyBehavior: "restart-required", default: 16, minimum: 1, maximum: 32 },
  } },
  uiSchema: { sections: [{ id: "general", title: "通用设置", localizations: { "zh-CN": "通用设置", en: "General" }, order: 1 }], fields: { worldName: { section: "general", order: 1, control: "text" }, worldSize: { section: "general", order: 2, control: "select" }, maxPlayers: { section: "general", order: 3, control: "number" } } },
};

const tmodManifest = {
  ...terrariaManifest,
  providerReleaseId: "prv_tmod",
  gameKey: "tmodloader",
  displayName: "tModLoader",
  gameVersions: ["v2026.07.3.0"],
  capabilities: ["configuration", "mods"],
  modCatalog: { entries: [
    { modId: "calamity", displayName: "Calamity Mod", versions: [{ version: "2.1", digest: "sha256:calamity", dependencies: [{ modId: "library", version: "1.0" }] }] },
    { modId: "library", displayName: "Mod Library", versions: [{ version: "1.0", digest: "sha256:library" }] },
  ] },
};

const terrariaDetailManifest = {
  ...terrariaManifest,
  configurationSchema: {
    required: ["worldName", "worldSize"],
    properties: {
      ...terrariaManifest.configurationSchema.properties,
      difficulty: { type: "enum", title: "难度", localizations: { "zh-CN": { title: "难度", enumLabels: { classic: "经典", expert: "专家", master: "大师", journey: "旅途" } }, en: { title: "Difficulty", enumLabels: { classic: "Classic", expert: "Expert", master: "Master", journey: "Journey" } } }, applyBehavior: "create-only", default: "classic", enum: ["classic", "expert", "master", "journey"] },
      motd: { type: "string", title: "欢迎语", localizations: { "zh-CN": { title: "欢迎语" }, en: { title: "Message of the day" } }, applyBehavior: "restart-required", default: "欢迎来到服务器" },
      password: { type: "secret", title: "服务器密码", localizations: { "zh-CN": { title: "服务器密码" }, en: { title: "Server password" } }, applyBehavior: "restart-required", default: "" },
      worldEvil: { type: "enum", title: "世界邪恶类型", localizations: { "zh-CN": { title: "世界邪恶类型", enumLabels: { random: "随机", corruption: "腐化", crimson: "猩红" } }, en: { title: "World evil", enumLabels: { random: "Random", corruption: "Corruption", crimson: "Crimson" } } }, applyBehavior: "create-only", default: "random", enum: ["random", "corruption", "crimson"] },
      secure: { type: "boolean", title: "安全模式", localizations: { "zh-CN": { title: "安全模式", description: "启用服务端安全检查" }, en: { title: "Secure mode", description: "Enable server-side security checks" } }, applyBehavior: "restart-required", default: true },
    },
  },
  uiSchema: {
    sections: terrariaManifest.uiSchema.sections,
    fields: {
      ...terrariaManifest.uiSchema.fields,
      difficulty: { section: "general", order: 4, control: "select" },
      motd: { section: "general", order: 5, control: "text" },
      password: { section: "general", order: 6, control: "password" },
      worldEvil: { section: "general", order: 7, control: "select" },
      secure: { section: "general", order: 8, control: "switch" },
    },
  },
};

async function mockCreateDependencies(page: import("@playwright/test").Page, walletAvailableMinor = 100_000) {
  await page.route("**/control-plane/v1/workspaces", (route) => route.fulfill({ json: [{ id: "ws_ember", slug: "ember", name: "Ember Realms" }] }));
  await page.route(/\/control-plane\/v1\/providers\/releases\?/, (route) => route.fulfill({ json: [
    { id: "prv_terraria", gameKey: "terraria", displayName: "Terraria", releaseVersion: "2", gameVersions: ["1.4.5.8"], capabilities: ["configuration"] },
    { id: "prv_tmod", gameKey: "tmodloader", displayName: "tModLoader", releaseVersion: "2", gameVersions: ["v2026.07.3.0"], capabilities: ["configuration", "mods"] },
  ] }));
  await page.route(/\/control-plane\/v1\/providers\/releases\/([^/]+)\/manifest\?/, (route) => route.fulfill({ json: route.request().url().includes("prv_tmod") ? tmodManifest : terrariaManifest }));
  await page.route(/\/control-plane\/v1\/regions\?/, (route) => route.fulfill({ json: [{ id: "reg_full", code: "full", name: "Full Region", names: { "zh-CN": "无容量区域", en: "Full region" }, available: false }, { id: "reg_asia_east", code: "asia-east", name: "Asia East", names: { "zh-CN": "亚洲东部", en: "Asia East" }, available: true }] }));
  await page.route(/\/control-plane\/v1\/regions\/reg_asia_east\/catalog\?/, (route) => route.fulfill({ json: { regionId: "reg_asia_east", currency: "CNY", resourceBounds: { cpuMilli: { minimum: 500, maximum: 8000, step: 500 }, memoryMiB: { minimum: 1024, maximum: 16384, step: 1024 }, diskGiB: { minimum: 10, maximum: 100, step: 5 } }, unitPrices: [], capacityState: "available", endpointDeliveryModes: ["node-direct"], dedicatedIpAvailable: false, catalogVersion: 1, priceBookId: "pb_1" } }));
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/wallet$/, (route) => route.fulfill({ json: { workspaceId: "ws_ember", currency: "CNY", promotionalMinor: walletAvailableMinor, cashMinor: 0, availableMinor: walletAvailableMinor, state: "active" } }));
}

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
  if (process.env.CAPTURE_PHASE72_QA === "1") {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.screenshot({ path: "qa/phase7-instance-list-1440.png", fullPage: true });
  }

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

test("create flow converts customer units and reuses the reviewed quote on retry", async ({ page }) => {
  await mockCreateDependencies(page);
  let quotePayload: unknown;
  const idempotencyKeys: string[] = [];
  let createAttempts = 0;
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/quotes$/, async (route) => {
    quotePayload = route.request().postDataJSON();
    await route.fulfill({ json: { id: "quote_reviewed", workspaceId: "ws_ember", regionId: "reg_asia_east", priceBookId: "pb_1", resourceSpec: { cpuMilli: 1500, memoryMiB: 3072, diskGiB: 25 }, currency: "CNY", estimatedHourlyMinor: 25, expiresAt: "2099-09-11T12:30:00Z" } });
  });
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/instances$/, async (route) => {
    createAttempts += 1;
    idempotencyKeys.push(route.request().headers()["idempotency-key"]);
    if (createAttempts === 1) { await route.fulfill({ status: 500, json: { error: "temporary" } }); return; }
    await route.fulfill({ status: 202, json: { instance: { id: "lin_created", workspaceId: "ws_ember", regionId: "reg_asia_east", name: "terraria-prod", providerReleaseId: "prv_terraria", gameVersion: "1.4.5.8", configurationRevisionId: "rev_1", resourceSpec: { cpuMilli: 1500, memoryMiB: 3072, diskGiB: 25 }, desiredState: "running", observedState: "pending", endpoints: [], latestOperationId: "op_create", createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, operation: { id: "op_create", kind: "create", resourceType: "instance", resourceId: "lin_created", status: "pending", steps: [], createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" } } });
  });

  await page.goto("/w/ember/instances/new");
  await expect(page.locator(".required-mark")).toHaveCount(3);
  await expect(page.getByLabel("实例名称")).toHaveAttribute("required", "");
  await expect(page.getByLabel("区域")).toHaveValue("reg_asia_east");
  await expect(page.getByLabel("区域").locator("option:checked")).toHaveText("亚洲东部");
  await page.getByLabel("实例名称").fill("terraria-prod");
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.locator(".required-mark")).toHaveCount(3);
  await expect(page.getByLabel("vCPU")).toHaveAttribute("required", "");
  await expect(page.getByLabel("vCPU")).toHaveValue("2");
  await page.getByLabel("vCPU").fill("1.5");
  await page.getByLabel("内存").fill("3");
  await page.getByLabel("磁盘").fill("25");
  await expect(page.getByText("MiB", { exact: false })).toHaveCount(0);
  await expect(page.getByText("vCPU (m)", { exact: true })).toHaveCount(0);
  if (process.env.CAPTURE_PHASE72_QA === "1") {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.screenshot({ path: "qa/phase7-create-resources-1440.png", fullPage: true });
  }
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.locator(".required-mark")).toHaveCount(2);
  await expect(page.getByLabel("世界名称")).toHaveAttribute("required", "");
  await expect(page.getByLabel("最大玩家数")).not.toHaveAttribute("required", "");
  await expect(page.getByLabel("世界名称")).toHaveValue("泰拉瑞亚世界");
  await expect(page.getByLabel("世界大小")).toHaveValue("medium");
  await expect(page.getByLabel("世界大小").locator("option:checked")).toHaveText("中型");
  if (process.env.CAPTURE_PHASE72_QA === "1") await page.screenshot({ path: "qa/phase7-create-config-1440.png", fullPage: true });
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.getByText("¥0.25/小时 · ¥6.00/24小时", { exact: true })).toBeVisible();
  await expect(page.getByText("连接地址将在部署完成后由系统分配", { exact: true })).toBeVisible();
  await expect(page.getByText(/报价有效至/)).toHaveCount(0);
  expect(quotePayload).toEqual({ regionId: "reg_asia_east", providerReleaseId: "prv_terraria", resourceSpec: { cpuMilli: 1500, memoryMiB: 3072, diskGiB: 25 } });
  for (const unsupported of ["公网端口", "协议", "带宽", "节点", "独立 IP"]) await expect(page.getByLabel(unsupported)).toHaveCount(0);
  if (process.env.CAPTURE_PHASE72_QA === "1") {
    await page.screenshot({ path: "qa/phase7-create-review-1440.png", fullPage: true });
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.screenshot({ path: "qa/phase7-create-review-1920.png", fullPage: true });
  }
  await page.setViewportSize({ width: 390, height: 844 });
  if (process.env.CAPTURE_PHASE72_QA === "1") await page.screenshot({ path: "qa/phase7-create-review-390.png", fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations.filter((item) => item.impact === "critical" || item.impact === "serious")).toEqual([]);

  await page.getByRole("button", { name: "创建并部署" }).click();
  await expect(page.getByText("创建请求未被接受，请检查配置与余额", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "创建并部署" }).click();
  await expect(page).toHaveURL(/\/w\/ember\/operations\/op_create\?instance=lin_created$/);
  expect(idempotencyKeys).toEqual(["create-quote_reviewed", "create-quote_reviewed"]);
});

test("provider-driven create flow exposes mods only when supported", async ({ page }) => {
  await mockCreateDependencies(page);
  await page.goto("/w/ember/instances/new");
  const modStep = page.getByRole("listitem").filter({ hasText: "模组" });
  await expect(modStep).toHaveCount(0);
  await page.getByLabel("游戏与版本").selectOption("prv_tmod");
  await expect(modStep).toBeVisible();
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.getByLabel("vCPU")).toBeVisible();
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.getByLabel("世界名称")).toHaveValue("泰拉瑞亚世界");
  await page.getByRole("button", { name: "下一步" }).click();
  await page.getByRole("checkbox", { name: "Calamity Mod" }).check();
  await expect(page.getByRole("checkbox", { name: "Mod Library" })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "Mod Library" })).toBeDisabled();
});

test("insufficient 24-hour balance blocks create submission", async ({ page }) => {
  await mockCreateDependencies(page, 500);
  let createRequests = 0;
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/quotes$/, (route) => route.fulfill({ json: { id: "quote_low_balance", workspaceId: "ws_ember", regionId: "reg_asia_east", priceBookId: "pb_1", resourceSpec: { cpuMilli: 2000, memoryMiB: 4096, diskGiB: 20 }, currency: "CNY", estimatedHourlyMinor: 25, expiresAt: "2099-09-11T12:30:00Z" } }));
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/instances$/, (route) => { createRequests += 1; return route.abort(); });

  await page.goto("/w/ember/instances/new");
  await page.getByRole("button", { name: "下一步" }).click();
  await page.getByRole("button", { name: "下一步" }).click();
  await page.getByRole("button", { name: "下一步" }).click();
  await expect(page.getByText("余额不足以覆盖预计 24 小时费用", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建并部署" })).toBeDisabled();
  expect(createRequests).toBe(0);
});

test("create flow follows the saved English locale without exposing provider values", async ({ context, page }) => {
  await context.addCookies([{ name: "gamepanel.locale", value: "en", domain: "127.0.0.1", path: "/" }]);
  await mockCreateDependencies(page);
  await page.goto("/w/ember/instances/new");
  await expect(page.getByRole("heading", { name: "Create instance" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Instances" })).toBeVisible();
  await expect(page.getByLabel("Region").locator("option:checked")).toHaveText("Asia East");
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.getByText("0.5–8 vCPU, step 0.5", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.getByLabel("World name")).toHaveValue("GamePanel World");
  await expect(page.getByLabel("World size")).toHaveValue("medium");
  await expect(page.getByLabel("World size").locator("option:checked")).toHaveText("Medium");
  await expect(page.getByText("服务器名称", { exact: true })).toHaveCount(0);
  await expect(page.getByText("medium", { exact: true })).toHaveCount(0);
});

test("configuration tolerates legacy revisions with a null mod lock", async ({ page }) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.route("**/control-plane/v1/workspaces", (route) => route.fulfill({ json: [{ id: "ws_ember", slug: "ember", name: "Ember Realms" }] }));
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/instances\/lin_legacy$/, (route) => route.fulfill({ json: {
    id: "lin_legacy", workspaceId: "ws_ember", regionId: "reg_asia_east", name: "legacy-terraria", providerReleaseId: "prv_terraria", gameVersion: "1.4.5.8", configurationRevisionId: "rev_legacy", resourceSpec: { cpuMilli: 1000, memoryMiB: 2048, diskGiB: 20 }, desiredState: "running", observedState: "running", endpoints: [], latestOperationId: "op_create", createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z",
  } }));
  await page.route(/\/control-plane\/v1\/providers\/releases\/prv_terraria\/manifest\?/, (route) => route.fulfill({ json: terrariaDetailManifest }));
  await page.route(/\/control-plane\/v1\/regions\?/, (route) => route.fulfill({ json: [{ id: "reg_asia_east", code: "asia-east", name: "Asia East", names: { "zh-CN": "亚洲东部", en: "Asia East" }, available: true }] }));
  await page.route(/\/control-plane\/v1\/workspaces\/ws_ember\/instances\/lin_legacy\/revisions\/rev_legacy$/, (route) => route.fulfill({ json: {
    id: "rev_legacy", operationId: "op_create", logicalInstanceId: "lin_legacy", providerReleaseId: "prv_terraria", gameVersion: "1.4.5.8", schemaVersion: 1, configuration: { worldName: "旧世界", worldSize: "medium", maxPlayers: 8, difficulty: "classic", motd: "欢迎来到服务器", password: "", worldEvil: "random", secure: true }, modLock: null, applyBehavior: "create-only", createdAt: "2026-09-11T00:00:00Z",
  } }));

  await page.goto("/w/ember/instances/lin_legacy?tab=overview");
  await expect(page.getByText("亚洲东部", { exact: true })).toBeVisible();
  if (process.env.CAPTURE_PHASE72_QA === "1") {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.screenshot({ path: "qa/phase7-instance-overview-1440.png", fullPage: true });
  }
  await page.goto("/w/ember/instances/lin_legacy?tab=configuration");
  await expect(page.getByLabel("世界名称")).toHaveValue("旧世界");
  await expect(page.getByText("配置已同步", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "保存并应用" })).toBeDisabled();
  if (process.env.CAPTURE_PHASE72_QA === "1") await page.screenshot({ path: "qa/phase7-instance-config-1440.png", fullPage: true });
  await page.getByLabel("世界名称").fill("新世界");
  await expect(page.getByText("有未保存的更改", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "保存并应用" })).toBeEnabled();
  if (process.env.CAPTURE_PHASE72_QA === "1") {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: "qa/phase7-instance-config-390.png", fullPage: true });
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations.filter((item) => item.impact === "critical" || item.impact === "serious")).toEqual([]);
  expect(pageErrors).toEqual([]);
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
