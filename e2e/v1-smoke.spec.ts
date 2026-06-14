import { expect, test, type Page } from "@playwright/test";

async function mockApi(page: Page) {
  const servers = [
    {
      id: "server-e2e",
      name: "E2E Terraria",
      providerKey: "terraria-tmodloader",
      status: "running",
      worldName: "E2E World",
      port: 17785,
      maxPlayers: 8,
      password: "secret",
      config: {
        serverName: "E2E Terraria",
        worldName: "E2E World",
        worldSize: "medium",
        difficulty: "classic",
        maxPlayers: 8,
        port: 17785,
        password: "secret",
        motd: "E2E ready",
        seed: "",
        secure: true,
        language: "en-US",
        autoCreateWorld: true
      }
    },
    {
      id: "server-target",
      name: "Target Terraria",
      providerKey: "terraria-vanilla",
      status: "stopped",
      worldName: "Target World",
      port: 17786,
      maxPlayers: 8,
      config: {
        serverName: "Target Terraria",
        worldName: "Target World",
        worldSize: "medium",
        difficulty: "classic",
        maxPlayers: 8,
        port: 17786,
        password: "",
        motd: "",
        seed: "",
        secure: true,
        language: "en-US",
        autoCreateWorld: true
      }
    }
  ];

  await page.route("**/healthz", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ status: "ok" })
    });
  });

  await page.route("**/api/runtime/docker/hosts", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        currentHost: "unix:///Users/pengwu/.orbstack/run/docker.sock",
        candidates: [
          {
            host: "unix:///Users/pengwu/.orbstack/run/docker.sock",
            label: "OrbStack Docker",
            source: "orbstack",
            exists: true,
            active: true
          }
        ]
      })
    });
  });

  await page.route("**/api/runtime/docker", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        available: true,
        message: "Docker daemon is available",
        host: "unix:///Users/pengwu/.orbstack/run/docker.sock",
        lastCheckedAt: new Date("2026-06-14T09:00:00.000Z").toISOString()
      })
    });
  });

  await page.route("**/api/activity", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify([
        {
          id: "activity-e2e",
          instanceId: "server-e2e",
          type: "server.started",
          message: "Started server E2E Terraria",
          createdAt: "2026-06-14T09:00:00.000Z"
        }
      ])
    });
  });

  await page.route("**/api/servers/server-e2e/logs", async (route) => {
    await route.fulfill({
      contentType: "text/event-stream",
      body: "event: log\ndata: Terraria Server v1.4.3.6\n\nevent: log\ndata: Listening on port 17785\n\nevent: log\ndata: Server started\n\n"
    });
  });

  await page.route("**/api/servers/server-e2e/command", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ status: "sent" })
    });
  });

  await page.route("**/api/servers/server-e2e/mods", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify([
        {
          id: "mod-e2e",
          instanceId: "server-e2e",
          fileName: "enabled.json",
          sizeBytes: 128,
          enabled: true,
          createdAt: "2026-06-14T09:00:00.000Z"
        }
      ])
    });
  });

  await page.route("**/api/servers/server-e2e/mods/mod-e2e", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ status: "deleted" })
    });
  });

  await page.route("**/api/servers/server-e2e/backups", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          id: "backup-new",
          instanceId: "server-e2e",
          fileName: "manual.zip",
          worldName: "E2E World",
          sizeBytes: 4096,
          type: "Manual",
          createdAt: "2026-06-14T09:05:00.000Z"
        })
      });
      return;
    }
    await route.fallback();
  });

  await page.route("**/api/servers/server-e2e", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(servers[0])
    });
  });

  await page.route("**/api/servers", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          id: "server-e2e",
          name: "E2E Terraria",
          providerKey: "terraria-vanilla",
          status: "stopped",
          worldName: "E2E World",
          port: 7777,
          maxPlayers: 8
        })
      });
      return;
    }

    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(servers)
    });
  });

  await page.route("**/api/worlds/world-e2e/migrate", async (route) => {
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({
        id: "world-e2e-migrated",
        instanceId: "server-target",
        name: "E2E World",
        fileName: "e2e.wld",
        sizeBytes: 2048,
        activeInstanceId: "server-target",
        createdAt: "2026-06-14T09:01:00.000Z"
      })
    });
  });

  await page.route("**/api/worlds", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify([
        {
          id: "world-e2e",
          instanceId: "server-e2e",
          name: "E2E World",
          fileName: "e2e.wld",
          sizeBytes: 2048,
          activeInstanceId: "server-e2e",
          createdAt: "2026-06-14T09:00:00.000Z"
        }
      ])
    });
  });

  await page.route("**/api/backups/backup-e2e/restore", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ status: "restored" })
    });
  });

  await page.route("**/api/backups/backup-e2e/migrate", async (route) => {
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({
        id: "backup-e2e-migrated",
        instanceId: "server-target",
        fileName: "e2e.zip",
        worldName: "Target World",
        sizeBytes: 4096,
        type: "Manual",
        createdAt: "2026-06-14T09:02:00.000Z"
      })
    });
  });

  await page.route("**/api/backups", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify([
        {
          id: "backup-e2e",
          instanceId: "server-e2e",
          fileName: "e2e.zip",
          worldName: "E2E World",
          sizeBytes: 4096,
          type: "Manual",
          createdAt: "2026-06-14T09:00:00.000Z"
        }
      ])
    });
  });

  await page.route("**/api/terraria/config/preview", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ serverconfig: "world=E2E World\nmaxplayers=8\nport=7777" })
    });
  });
}

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test("app shell renders Chinese UI, game art, avatar, and Docker scan feedback", async ({ page }) => {
  await page.goto("/dashboard");

  await expect(page).toHaveTitle(/GamePanel Lite/);
  await expect(page.getByRole("heading", { name: "仪表盘" })).toBeVisible();
  await expect(page.getByRole("link", { name: /GamePanel Lite/ })).toBeVisible();
  await expect(page.getByAltText("Terraria 官方游戏封面").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "用户资料" })).toBeVisible();
  await expect(page.getByRole("banner")).toContainText("在线");
  await expect(page.getByRole("banner").getByRole("link", { name: "创建服务器" })).toBeVisible();

  await page.getByRole("link", { name: "设置" }).click();
  await expect(page).toHaveURL(/\/settings$/, { timeout: 15_000 });
  await expect(page.getByRole("heading", { name: "设置" })).toBeVisible();
  await expect(page.getByText("Docker 已连接，可以创建和管理服务器。")).toBeVisible();

  await page.getByRole("button", { name: /扫描/ }).click();
  await expect(page.getByText("扫描完成，发现 1 个候选 Docker Host。")).toBeVisible();

  await page.getByRole("button", { name: /更改/ }).click();
  await expect(page.getByText("候选 Host")).toBeVisible();
  await expect(page.getByRole("button", { name: /OrbStack Docker/ })).toBeVisible();
});

test("create server wizard keeps clicked mode and preset selected", async ({ page }) => {
  await page.goto("/servers/new");

  await expect(page.getByRole("heading", { name: "创建 Terraria 服务器" })).toBeVisible();
  await expect(page.getByRole("main").getByAltText("Terraria 官方游戏封面")).toBeVisible();

  await page.getByRole("button", { name: /模式/ }).click();
  const vanilla = page.getByRole("button", { name: /原版 Terraria/ });
  const tmod = page.getByRole("button", { name: /tModLoader/ });

  await vanilla.click();
  await expect(vanilla).toHaveAttribute("aria-pressed", "true");
  await expect(tmod).toHaveAttribute("aria-pressed", "false");

  await page.getByRole("button", { name: "3 预设" }).click();
  const expert = page.getByRole("button", { name: /专家冒险/ });
  const building = page.getByRole("button", { name: /建筑世界/ });

  await expert.click();
  await expect(expert).toHaveAttribute("aria-pressed", "true");
  await expect(building).toHaveAttribute("aria-pressed", "false");

  await building.hover();
  await expect(expert).toHaveAttribute("aria-pressed", "true");
  await expect(building).toHaveAttribute("aria-pressed", "false");
});

test("server detail and management flows expose live V1 actions", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);

  await page.goto("/servers/server-e2e");

  await expect(page.getByRole("heading", { name: "E2E Terraria" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "概览" })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("heading", { name: "加入服务器" })).toBeVisible();

  await page.getByRole("tab", { name: "控制台" }).click();
  await expect(page.getByText("Listening on port 17785")).toBeVisible();
  await expect(page.getByText("Server started")).toBeVisible();

  await page.getByRole("button", { name: "复制邀请文本" }).click();
  await expect(page.getByRole("button", { name: "已复制" })).toBeVisible();
  await expect(page.evaluate(() => navigator.clipboard.readText())).resolves.toContain("127.0.0.1:17785");

  const commandRequest = page.waitForRequest((request) => request.method() === "POST" && request.url().includes("/api/servers/server-e2e/command"));
  await page.getByPlaceholder("输入命令...").fill("say hello");
  await page.getByRole("button", { name: "发送" }).click();
  await commandRequest;
  await expect(page.getByText("> say hello")).toBeVisible();

  await page.getByRole("tab", { name: "配置" }).click();
  await expect(page.getByText("Preview serverconfig.txt")).not.toBeVisible();
  await expect(page.getByText("world=E2E World")).toBeVisible();

  await page.getByRole("tab", { name: "世界" }).click();
  await expect(page.getByRole("heading", { name: "当前服务器世界" })).toBeVisible();
  await expect(page.getByText("E2E World").first()).toBeVisible();

  await page.getByRole("tab", { name: "备份" }).click();
  await expect(page.getByRole("heading", { name: "当前服务器备份" })).toBeVisible();
  const detailBackup = page.waitForRequest((request) => request.method() === "POST" && request.url().includes("/api/servers/server-e2e/backups"));
  await page.getByRole("button", { name: "创建当前备份" }).click();
  await detailBackup;
  await page.getByRole("button", { name: "恢复" }).click();
  await expect(page.getByRole("dialog", { name: /恢复备份/ })).toBeVisible();
  const detailRestore = page.waitForRequest((request) => request.method() === "POST" && request.url().includes("/api/backups/backup-e2e/restore"));
  await page.getByRole("dialog").getByRole("button", { name: "恢复" }).click();
  await detailRestore;

  await page.getByRole("tab", { name: "模组" }).click();
  await expect(page.getByRole("heading", { name: "当前服务器模组" })).toBeVisible();
  await expect(page.getByText("enabled.json")).toBeVisible();
  await page.getByRole("button", { name: "删除" }).click();
  await expect(page.getByRole("dialog", { name: /删除模组文件/ })).toBeVisible();

  await page.goto("/worlds");
  await expect(page.getByRole("heading", { name: "世界" })).toBeVisible();
  await page.getByRole("combobox").selectOption("server-target");
  const worldMigration = page.waitForRequest((request) => request.method() === "POST" && request.url().includes("/api/worlds/world-e2e/migrate"));
  await page.getByRole("button", { name: "迁移" }).click();
  await worldMigration;

  await page.goto("/backups");
  await expect(page.getByRole("heading", { name: "备份" })).toBeVisible();
  await page.getByRole("button", { name: "恢复" }).click();
  await expect(page.getByRole("dialog", { name: /恢复备份/ })).toBeVisible();
  const restoreRequest = page.waitForRequest((request) => request.method() === "POST" && request.url().includes("/api/backups/backup-e2e/restore"));
  await page.getByRole("dialog").getByRole("button", { name: "恢复" }).click();
  await restoreRequest;
});
