import { expect, test, type Page } from "@playwright/test";

async function mockConsoleAPIs(
  page: Page,
  userName = "admin",
  opts: { isAdmin?: boolean; tenantRole?: string } = {},
) {
  const isAdmin = opts.isAdmin ?? true;
  const tenantRole = opts.tenantRole;
  await page.route("**/apis/iam.gamepanel.io/v1/sessions/sess-e2e-01", async (route) => {
    if (route.request().method() === "DELETE") {
      await route.fulfill({ status: 204 });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "iam.gamepanel.io/v1",
        kind: "Session",
        metadata: {
          name: "sess-e2e-01",
          annotations: { "iam.gamepanel.io/current": "true" },
        },
        user: userName,
      }),
    });
  });

  await page.route("**/apis/iam.gamepanel.io/v1/sessions", async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "iam.gamepanel.io/v1",
        kind: "SessionList",
        metadata: { totalCount: 1 },
        items: [
          {
            apiVersion: "iam.gamepanel.io/v1",
            kind: "Session",
            metadata: {
              name: "sess-e2e-01",
              annotations: { "iam.gamepanel.io/current": "true" },
            },
            user: userName,
          },
        ],
      }),
    });
  });

  await page.route(`**/apis/iam.gamepanel.io/v1/users/${userName}`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "iam.gamepanel.io/v1",
        kind: "User",
        metadata: { name: userName },
        spec: { email: `${userName}@example.com`, state: "Active" },
        status: { state: "Active" },
      }),
    });
  });

  await page.route("**/apis/core.gamepanel.io/v1/tenants", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "core.gamepanel.io/v1",
        kind: "TenantList",
        metadata: { totalCount: 1 },
        items: [
          {
            apiVersion: "core.gamepanel.io/v1",
            kind: "Tenant",
            metadata: { name: "studio-alpha" },
            spec: {
              displayName: "Studio Alpha",
              type: "Organization",
              owner: "admin",
              quota: {
                cpu: "16",
                memory: "32Gi",
                storage: "200Gi",
                instances: "20",
                backups: "10",
                backupStorage: "100Gi",
              },
            },
            status: { phase: "Active", usage: { instances: "1", cpu: "2" } },
          },
        ],
      }),
    });
  });

  await page.route("**/apis/core.gamepanel.io/v1/runtimeclasses", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "core.gamepanel.io/v1",
        kind: "RuntimeClassList",
        metadata: { totalCount: 1 },
        items: [
          {
            apiVersion: "core.gamepanel.io/v1",
            kind: "RuntimeClass",
            metadata: { name: "docker" },
            handler: "docker",
            isDefault: true,
          },
        ],
      }),
    });
  });

  await page.route("**/apis/core.gamepanel.io/v1/apps", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "core.gamepanel.io/v1",
        kind: "AppList",
        metadata: { totalCount: 1 },
        items: [
          {
            apiVersion: "core.gamepanel.io/v1",
            kind: "App",
            metadata: { name: "minecraft-paper" },
            displayName: "Minecraft",
            category: "sandbox",
            image: "gamepanel/runtime:latest",
            coverImage: "/landing/games/minecraft-paper.jpg",
            published: true,
          },
        ],
      }),
    });
  });

  await page.route("**/apis/core.gamepanel.io/v1/nodes*", async (route) => {
    if (!isAdmin) {
      await route.fulfill({
        status: 403,
        contentType: "application/json",
        body: JSON.stringify({ code: 403, message: "Forbidden" }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "core.gamepanel.io/v1",
        kind: "NodeList",
        metadata: { totalCount: 1 },
        items: [
          {
            apiVersion: "core.gamepanel.io/v1",
            kind: "Node",
            metadata: { name: "node-ap-01" },
            spec: { unschedulable: false },
            status: {
              phase: "Ready",
              addresses: [
                { type: "InternalIP", address: "10.0.1.12" },
                { type: "ExternalIP", address: "203.0.113.10" },
              ],
              allocatable: { cpu: "16", memory: "64Gi", storage: "500Gi" },
              runtimes: [{ name: "docker", ready: true }],
            },
          },
        ],
      }),
    });
  });

  const mcInstance = {
    apiVersion: "core.gamepanel.io/v1",
    kind: "Instance",
    metadata: {
      name: "mc-survival-01",
      tenant: "studio-alpha",
      labels: { "gamepanel.io/app": "minecraft-paper" },
    },
    spec: {
      runtimeClassName: "docker",
      nodeName: "node-ap-01",
      state: "Running",
      image: "gamepanel/runtime:latest",
      resources: { requests: { cpu: "2", memory: "4Gi", storage: "20Gi" } },
    },
    status: {
      phase: "Running",
      endpoint: {
        hostIP: "203.0.113.10",
        ports: [
          {
            name: "game",
            protocol: "TCP",
            targetPort: 25565,
            publishedPort: 31245,
          },
        ],
      },
    },
  };

  await page.route("**/apis/core.gamepanel.io/v1/tenants/*/instances*", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/metrics")) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ series: [] }),
      });
      return;
    }
    if (url.pathname.endsWith("/mc-survival-01")) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mcInstance),
      });
      return;
    }
    if (url.searchParams.get("watch") === "true") {
      await route.fulfill({
        status: 200,
        contentType: "application/x-ndjson",
        body: JSON.stringify({ type: "MODIFIED", object: mcInstance }) + "\n",
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "core.gamepanel.io/v1",
        kind: "InstanceList",
        metadata: { totalCount: 1 },
        items: [mcInstance],
      }),
    });
  });

  await page.route("**/apis/core.gamepanel.io/v1/tenants/*/alerts*", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ apiVersion: "core.gamepanel.io/v1", kind: "AlertList", items: [] }),
    });
  });

  await page.route("**/apis/registry.gamepanel.io/v1/artifacts*", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ apiVersion: "registry.gamepanel.io/v1", kind: "ArtifactList", items: [] }),
    });
  });

  await page.route("**/apis/iam.gamepanel.io/v1/users/*/messages*", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [], unreadCount: 0 }),
    });
  });

  await page.route("**/apis/iam.gamepanel.io/v1/users", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "iam.gamepanel.io/v1",
        kind: "UserList",
        metadata: { totalCount: 1 },
        items: [{ metadata: { name: userName }, spec: { email: `${userName}@example.com`, state: "Active" } }],
      }),
    });
  });

  await page.route("**/apis/iam.gamepanel.io/v1/globalroles", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "iam.gamepanel.io/v1",
        kind: "GlobalRoleList",
        metadata: { totalCount: 1 },
        items: [
          {
            metadata: { name: "platform-admin" },
            description: "Full cluster administrator",
            rules: [{ apiGroups: ["*"], resources: ["*"], verbs: ["*"] }],
          },
        ],
      }),
    });
  });

  await page.route("**/apis/iam.gamepanel.io/v1/globalrolebindings", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "iam.gamepanel.io/v1",
        kind: "GlobalRoleBindingList",
        metadata: { totalCount: 1 },
        items: [
          {
            metadata: { name: `${userName}-platform-admin` },
            roleRef: { kind: "GlobalRole", name: "platform-admin" },
            subjects: [{ kind: "User", name: userName }],
          },
        ],
      }),
    });
  });

  await page.route("**/apis/iam.gamepanel.io/v1/tenants/*/tenantrolebindings", async (route) => {
    const items = tenantRole
      ? [
          {
            apiVersion: "iam.gamepanel.io/v1",
            kind: "TenantRoleBinding",
            metadata: { name: `member-${userName}`, tenant: "studio-alpha" },
            roleRef: { name: tenantRole },
            subjects: [{ kind: "User", name: userName }],
          },
        ]
      : [];
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        apiVersion: "iam.gamepanel.io/v1",
        kind: "TenantRoleBindingList",
        metadata: { totalCount: items.length },
        items,
      }),
    });
  });
}

test.describe("GamePanel Lite V1 Declarative Console", () => {
  test("redirects unauthenticated user to /login and authenticates via iam.gamepanel.io/v1/sessions", async ({
    page,
  }) => {
    await page.route("**/apis/iam.gamepanel.io/v1/sessions", async (route) => {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "iam.gamepanel.io/v1",
          kind: "Session",
          metadata: { name: "sess-e2e-01" },
          subjectKind: "User",
          username: "admin",
          token: "jwt-e2e-token",
          user: "admin",
          groups: ["system:authenticated"],
        }),
      });
    });
    await mockConsoleAPIs(page, "admin");

    await page.goto("/console");
    await expect(page).toHaveURL(/\/login\?next=%2Fconsole/);
    await expect(page.getByRole("heading", { name: "欢迎回来" })).toBeVisible();
    await expect(page.locator("#username")).toHaveValue("");

    await page.locator("#username").fill("admin");
    await page.locator("#password").fill("AdminPass123!");
    await page.getByRole("button", { name: "登录" }).click();

    await expect(page).toHaveURL("/console");
    await expect(page.getByRole("heading", { name: "我的服务器" })).toBeVisible();
    await expect(page.getByText("mc-survival-01")).toBeVisible();
    await expect(page.getByText("203.0.113.10:31245")).toBeVisible();
  });

  test("registers collaborator via workspace invite link and enters console", async ({ page }) => {
    const payload = Buffer.from(JSON.stringify({ t: "studio-alpha" })).toString("base64url");
    const fakeToken = `${payload}.sig123`;

    await page.route(`**/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites/${fakeToken}`, async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            apiVersion: "core.gamepanel.io/v1",
            kind: "TenantInvite",
            metadata: { name: "inv-01", tenant: "studio-alpha" },
            tenantDisplayName: "Studio Alpha",
            inviter: "admin",
            role: "instance-operator",
          }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "core.gamepanel.io/v1",
          kind: "TenantInvite",
          metadata: { name: "inv-01", tenant: "studio-alpha" },
          username: "steve01",
          sessionToken: "jwt-e2e-token",
        }),
      });
    });
    await mockConsoleAPIs(page, "steve01");

    await page.goto(`/invite/${fakeToken}`);
    await expect(page.getByRole("heading", { name: "Studio Alpha" })).toBeVisible();
    await expect(page.getByText("运维", { exact: true })).toBeVisible();

    await page.locator("#inv-email").fill("steve01@example.com");
    await page.locator("#inv-username").fill("steve01");
    await page.locator("#inv-password").fill("CollaboratorPass123!");
    await page.locator("#inv-password-confirm").fill("CollaboratorPass123!");
    await page.getByRole("button", { name: "注册并加入" }).click();

    await expect(page).toHaveURL(/\/console(?:\?tenant=studio-alpha)?$/);
    await expect(page.getByText("mc-survival-01")).toBeVisible();
  });

  test("allows instance-operator to operate server tabs without create or delete server access", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("gp_session_token", "jwt-e2e-token");
      localStorage.setItem("gp_session_user", "steve01");
      localStorage.setItem("gp_session_name", "sess-e2e-01");
      localStorage.setItem("gp_selected_tenant", "studio-alpha");
    });
    await mockConsoleAPIs(page, "steve01", { isAdmin: false, tenantRole: "instance-operator" });

    await page.goto("/console");
    await expect(page.getByRole("heading", { name: "我的服务器" })).toBeVisible();
    await expect(page.getByText("mc-survival-01")).toBeVisible();
    await expect(page.getByRole("link", { name: /新建服务器/ })).toHaveCount(0);

    await page.goto("/console/servers/mc-survival-01");
    await expect(page.getByRole("heading", { name: "mc-survival-01" })).toBeVisible();
    for (const label of ["文件", "备份", "定时任务", "网络", "设置"]) {
      await expect(page.getByRole("tab", { name: label })).toBeVisible();
    }

    await page.getByRole("tab", { name: "设置" }).click();
    await expect(page.getByRole("button", { name: "保存" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "危险操作" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /删除服务器/ })).toHaveCount(0);
  });

  test("supports prewarm task log modal, toolbox log infinite scroll, and L4D2 workshop tab", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("gp_session_token", "jwt-e2e-token");
      localStorage.setItem("gp_session_user", "admin");
      localStorage.setItem("gp_session_name", "sess-e2e-01");
      localStorage.setItem("gp_selected_tenant", "studio-alpha");
    });
    await mockConsoleAPIs(page, "admin", { isAdmin: true });

    await page.route("**/apis/core.gamepanel.io/v1/nodes*", async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname.endsWith("/metrics")) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ samples: [] }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "core.gamepanel.io/v1",
          kind: "NodeList",
          metadata: { totalCount: 1 },
          items: [
            {
              apiVersion: "core.gamepanel.io/v1",
              kind: "Node",
              metadata: { name: "node-ap-01" },
              spec: { unschedulable: false },
              status: {
                phase: "Ready",
                addresses: [
                  { type: "InternalIP", address: "10.0.1.12" },
                  { type: "ExternalIP", address: "203.0.113.10" },
                ],
                allocatable: { cpu: "16", memory: "64Gi", storage: "500Gi" },
                capacity: { cpu: "16", memory: "64Gi", storage: "500Gi" },
                runtimes: [{ name: "docker", ready: true }],
                cachedArtifacts: ["l4d2-server.l4d2.latest"],
              },
            },
          ],
        }),
      });
    });

    await page.route("**/apis/core.gamepanel.io/v1/prewarmjobs*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "core.gamepanel.io/v1",
          kind: "PrewarmJobList",
          items: [
            {
              apiVersion: "core.gamepanel.io/v1",
              kind: "PrewarmJob",
              metadata: { name: "prewarm-l4d2" },
              spec: {
                nodeNames: ["node-ap-01"],
                artifacts: [{ repository: "l4d2-server", package: "l4d2", version: "latest" }],
              },
              status: {
                phase: "Succeeded",
                nodes: [
                  {
                    nodeName: "node-ap-01",
                    phase: "Succeeded",
                    message: "预热完成",
                    completedAt: "2026-11-16T12:00:00Z",
                  },
                ],
              },
            },
          ],
        }),
      });
    });

    await page.route("**/apis/registry.gamepanel.io/v1/artifacts*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "registry.gamepanel.io/v1",
          kind: "ArtifactList",
          items: [
            {
              apiVersion: "registry.gamepanel.io/v1",
              kind: "Artifact",
              metadata: { name: "l4d2-server.l4d2.latest" },
              spec: { repository: "l4d2-server", package: "l4d2", version: "latest" },
              status: { phase: "Ready", stage: "Completed", progress: 100 },
            },
          ],
        }),
      });
    });

    await page.routeWebSocket("**/apis/registry.gamepanel.io/v1/artifacts/*/log*", (ws) => {
      ws.send("SteamCMD initializing\nDownloading L4D2 build OK\n");
    });

    // 1. Prewarm task log viewer on Admin Nodes page (#960)
    await page.goto("/console/admin/nodes?node=node-ap-01&tab=cache");
    await expect(page.getByRole("heading", { name: "prewarm-l4d2" })).toHaveCount(0);
    await expect(page.getByText("node-ap-01").first()).toBeVisible();
    await page.getByRole("button", { name: "查看日志" }).first().click();
    await expect(page.getByText("Downloading L4D2 build OK")).toBeVisible();
    await page.getByRole("button", { name: "关闭" }).last().click();

    // 2. FloatingToolbox infinite scroll log pagination (#961 / #964)
    await page.route("**/apis/core.gamepanel.io/v1/alerts*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ apiVersion: "core.gamepanel.io/v1", kind: "AlertList", items: [] }),
      });
    });
    await page.route("**/apis/core.gamepanel.io/v1/events*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ apiVersion: "core.gamepanel.io/v1", kind: "EventList", items: [] }),
      });
    });

    const requestedOffsets: number[] = [];
    const nowMs = Date.now();
    await page.route("**/apis/core.gamepanel.io/v1/logs*", async (route) => {
      const url = new URL(route.request().url());
      const offset = Number(url.searchParams.get("offset") || "0");
      const limit = Number(url.searchParams.get("limit") || "100");
      requestedOffsets.push(offset);
      const total = 150;
      const count = Math.max(0, Math.min(limit, total - offset));
      const items = Array.from({ length: count }, (_, idx) => {
        const n = offset + idx;
        return {
          metadata: { name: `log-${n}`, creationTimestamp: new Date(nowMs - n * 1000).toISOString() },
          timestamp: new Date(nowMs - n * 1000).toISOString(),
          level: "INFO",
          service: "apiserver",
          message: `log-entry-${n}`,
          fields: { node: "node-ap-01" },
        };
      });
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "core.gamepanel.io/v1",
          kind: "LogEntryList",
          metadata: { totalCount: total, continue: offset + count < total ? String(offset + count) : "" },
          items,
        }),
      });
    });

    await page.goto("/console/admin/observability?tab=logs");
    await expect(page.getByText("已加载 100 / 共 150 条日志")).toBeVisible();
    expect(requestedOffsets).toContain(0);

    const logScrollBox = page
      .locator("div.overflow-y-auto")
      .filter({ has: page.getByText("日志时间分布") })
      .first();
    await logScrollBox.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
      el.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    await expect(page.getByText("共 150 条日志")).toBeVisible();
    await expect(page.getByText("log-entry-149")).toBeVisible();
    expect(requestedOffsets).toContain(100);

    // 3. Left 4 Dead 2 Workshop addon tab, switch staged badge, and addon_maps options
    const l4d2App = {
      apiVersion: "core.gamepanel.io/v1",
      kind: "App",
      metadata: { name: "left-4-dead-2" },
      displayName: "求生之路 2 Left 4 Dead 2",
      category: "survival",
      image: "gamepanel/runtime:latest",
      published: true,
      addonBinding: {
        repository: "l4d2-workshop",
        installMode: "flat_dir",
        targetDir: "left4dead2/addons",
      },
      configSchema: [
        {
          name: "startMap",
          label: "初始启动战役地图",
          type: "select",
          optionsSource: "addon_maps",
          default: "c1m1_hotel",
          envKey: "START_MAP",
          group: "基础设置",
          options: [{ value: "c1m1_hotel", label: "死亡中心" }],
        },
      ],
      consoleActions: [
        {
          id: "l4d2-changelevel",
          title: "切换战役地图",
          group: "地图难度",
          command: "changelevel {{map}}",
          starred: true,
          params: [
            {
              name: "map",
              label: "起始地图",
              type: "select",
              optionsSource: "addon_maps",
              defaultValue: "c1m1_hotel",
              options: [
                { value: "c1m1_hotel", label: "死亡中心" },
                { value: "c2m1_highway", label: "黑色狂欢节" },
                { value: "c3m1_plankcountry", label: "沼泽激战" },
                { value: "c5m1_waterfront", label: "教区" },
                { value: "c8m1_apartment", label: "毫不留情" },
              ],
            },
          ],
        },
      ],
    };
    const l4d2Instance = {
      apiVersion: "core.gamepanel.io/v1",
      kind: "Instance",
      metadata: {
        name: "l4d2-demo-01",
        tenant: "studio-alpha",
        labels: { "gamepanel.io/app": "left-4-dead-2" },
      },
      spec: {
        runtimeClassName: "docker",
        nodeName: "node-ap-01",
        state: "Running",
        image: "gamepanel/runtime:latest",
        resources: { requests: { cpu: "2", memory: "2Gi", storage: "15Gi" } },
        addons: [
          {
            repository: "l4d2-workshop",
            package: "121086524",
            version: "1358581160",
            title: "Urban Flight",
            installMode: "flat_dir",
            targetDir: "left4dead2/addons",
          },
        ],
      },
      status: { phase: "Running" },
    };

    await page.route("**/apis/core.gamepanel.io/v1/apps", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ apiVersion: "core.gamepanel.io/v1", kind: "AppList", items: [l4d2App] }),
      });
    });
    await page.route("**/apis/registry.gamepanel.io/v1/artifacts*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "registry.gamepanel.io/v1",
          kind: "ArtifactList",
          items: [
            {
              apiVersion: "registry.gamepanel.io/v1",
              kind: "Artifact",
              metadata: { name: "l4d2-workshop.121086524.1358581160" },
              spec: { repository: "l4d2-workshop", package: "121086524", version: "1358581160" },
              status: {
                phase: "Ready",
                stage: "Completed",
                progress: 100,
                mapFolders: ["uf1_boulevard", "uf2_rooftops"],
              },
            },
          ],
        }),
      });
    });
    await page.route("**/apis/core.gamepanel.io/v1/tenants/*/instances*", async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname.endsWith("/metrics")) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ samples: [] }),
        });
        return;
      }
      if (url.pathname.endsWith("/l4d2-demo-01")) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(l4d2Instance),
        });
        return;
      }
      if (url.searchParams.get("watch") === "true") {
        await route.fulfill({
          status: 200,
          contentType: "application/x-ndjson",
          body: JSON.stringify({ type: "MODIFIED", object: l4d2Instance }) + "\n",
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "core.gamepanel.io/v1",
          kind: "InstanceList",
          metadata: { totalCount: 1 },
          items: [l4d2Instance],
        }),
      });
    });
    await page.route("**/apis/registry.gamepanel.io/v1/packages*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "registry.gamepanel.io/v1",
          kind: "PackageList",
          metadata: { totalCount: 1 },
          items: [
            {
              metadata: { name: "l4d2-workshop.121086524" },
              repository: "l4d2-workshop",
              package: "121086524",
              title: "Urban Flight",
              summary: "Classic custom L4D2 campaign map",
              downloads: 420000,
              latestVersion: "1358581160",
            },
          ],
        }),
      });
    });

    await page.goto("/console/servers/l4d2-demo-01");
    await expect(page.getByRole("tab", { name: "模组" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "插件" })).toHaveCount(0);
    await page.getByRole("tab", { name: "模组" }).click();
    await expect(page.getByText("Urban Flight").first()).toBeVisible();

    // Toggling enable/disable switch marks the addon row as "待保存"
    await page.getByRole("switch", { name: "停用 Urban Flight" }).click();
    await expect(page.getByText("待保存")).toBeVisible();
    await page.getByRole("button", { name: "撤销" }).click();

    await page.getByRole("button", { name: "浏览创意工坊" }).first().click();
    await expect(page.getByRole("dialog").getByText("Urban Flight")).toBeVisible();
    await page.keyboard.press("Escape");

    // Settings -> startMap dropdown includes addon_maps from installed workshop VPK
    await page.getByRole("tab", { name: "设置" }).click();
    await expect(page.locator('#game-cfg-startMap option[value="uf1_boulevard"]')).toHaveCount(1);

    // FloatingToolbox -> 快捷指令 -> 切换战役地图 renders Select dropdown with uf1_boulevard
    await page.getByRole("button", { name: "打开工具箱" }).click();
    await page.getByRole("button", { name: /快捷指令/ }).click();
    await expect(page.getByLabel("起始地图").locator('option[value="uf1_boulevard"]')).toHaveCount(1);
  });

  test("hides node selector in create wizard, streamlines advanced settings, and toggles region automatically", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("gp_session_token", "jwt-e2e-token");
      localStorage.setItem("gp_session_user", "admin");
      localStorage.setItem("gp_session_name", "sess-e2e-01");
      localStorage.setItem("gp_selected_tenant", "studio-alpha");
    });
    await mockConsoleAPIs(page, "admin", { isAdmin: true });

    let nodeRegions = ["ap-east-1", "ap-east-1"];
    await page.route("**/apis/core.gamepanel.io/v1/nodes*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          apiVersion: "core.gamepanel.io/v1",
          kind: "NodeList",
          metadata: { totalCount: 2 },
          items: [
            {
              apiVersion: "core.gamepanel.io/v1",
              kind: "Node",
              metadata: { name: "home-vm01", labels: { "topology.gamepanel.io/region": nodeRegions[0] } },
              spec: { unschedulable: false },
              status: { phase: "Ready", runtimes: [{ name: "docker", ready: true }] },
            },
            {
              apiVersion: "core.gamepanel.io/v1",
              kind: "Node",
              metadata: { name: "node-um773", labels: { "topology.gamepanel.io/region": nodeRegions[1] } },
              spec: { unschedulable: false },
              status: { phase: "Ready", runtimes: [{ name: "docker", ready: true }] },
            },
          ],
        }),
      });
    });

    const createdPayloads: Array<{ spec?: { nodeSelector?: Record<string, string>; image?: string; ports?: Array<{ publishedPort?: number }> } }> = [];
    await page.route("**/apis/core.gamepanel.io/v1/tenants/*/instances*", async (route) => {
      const req = route.request();
      if (req.method() === "POST") {
        const body = req.postDataJSON() as (typeof createdPayloads)[number];
        createdPayloads.push(body);
        await route.fulfill({
          status: 201,
          contentType: "application/json",
          body: JSON.stringify({
            apiVersion: "core.gamepanel.io/v1",
            kind: "Instance",
            metadata: { name: "mc-test-01", tenant: "studio-alpha" },
            spec: body.spec,
            status: { phase: "Pending" },
          }),
        });
        return;
      }
      await route.fallback();
    });

    // 1. Single-region cluster: no 调度节点, no 部署地域, streamlined 高级设置
    await page.goto("/console/new");
    await page.getByRole("button", { name: /Minecraft/ }).click();
    await page.getByRole("button", { name: "下一步" }).click();
    await page.getByRole("button", { name: "下一步" }).click();

    await expect(page.getByText("调度节点")).toHaveCount(0);
    await expect(page.getByText("部署地域")).toHaveCount(0);
    await expect(page.locator("#adv-node")).toHaveCount(0);
    await expect(page.locator("#custom-image")).toHaveCount(0);

    await page.getByRole("button", { name: /高级设置/ }).click();
    await expect(page.getByRole("tab", { name: "自动分配" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByText("环境变量", { exact: true })).toBeVisible();
    await expect(page.getByText("启动命令")).toHaveCount(0);
    await page.getByRole("tab", { name: "指定端口" }).click();
    await expect(page.locator("#adv-published")).toHaveValue("25565");
    await page.getByRole("tab", { name: "自动分配" }).click();

    await page.getByRole("button", { name: "下一步" }).click();
    await expect(page.getByText("调度节点")).toHaveCount(0);
    await expect(page.getByText("连接端口", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "创建服务器" }).click();

    await expect.poll(() => createdPayloads.length).toBe(1);
    expect(createdPayloads[0]?.spec?.nodeSelector).toBeUndefined();
    expect(createdPayloads[0]?.spec?.image).toBe("gamepanel/runtime:latest");
    expect(createdPayloads[0]?.spec?.ports?.[0]?.publishedPort).toBe(0);

    // 2. Multi-region cluster: 部署地域 appears and sends topology.gamepanel.io/region
    nodeRegions = ["ap-east-1", "cn-south-1"];
    await page.goto("/console/new");
    await page.getByRole("button", { name: /Minecraft/ }).click();
    await page.getByRole("button", { name: "下一步" }).click();
    await page.getByRole("button", { name: "下一步" }).click();

    await expect(page.locator("#server-region")).toBeVisible();
    await expect(page.locator("#server-region option")).toHaveCount(3);
    await page.locator("#server-region").selectOption("cn-south-1");

    await page.getByRole("button", { name: "下一步" }).click();
    await expect(page.getByText("华南 1")).toBeVisible();
    await page.getByRole("button", { name: "创建服务器" }).click();

    await expect.poll(() => createdPayloads.length).toBe(2);
    expect(createdPayloads[1]?.spec?.nodeSelector).toEqual({
      "topology.gamepanel.io/region": "cn-south-1",
    });
  });
});
