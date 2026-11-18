"use client";

import { FormEvent, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useSearchParams } from "next/navigation";
import {
  ArrowLeft,
  Download,
  FileCode2,
  ImagePlus,
  Pencil,
  Plus,
  RotateCcw,
  Search,
  Trash2,
} from "lucide-react";
import { useConsole } from "../../ConsoleProvider";
import { errorMessage } from "../../lib/session";
import {
  DEFAULT_GAME_COVER,
  compressCoverImageFile,
  defaultGameCover,
  formatAppCategory,
  gameCover,
  resolveRichApp,
} from "../../lib/presets";
import type {
  AppAddonBinding,
  AppConfigField,
  AppConfigPreset,
  AppConsoleAction,
  AppConsoleTarget,
  AppFileTemplate,
  AppItem,
  AppMetricSpec,
  AppServerBinding,
  AppStoragePolicy,
  InstancePort,
} from "../../lib/types";
import { GameVersionsPanel } from "../../../components/console/admin/GameVersionsPanel";
import { validateName } from "../../../components/console/admin/validation";
import { AppConsoleActionsEditor } from "../../../components/console/servers/AppConsoleActionsEditor";
import { AppMetricsEditor } from "../../../components/console/servers/AppMetricsEditor";
import { GameConfigEditor } from "../../../components/console/servers/GameConfigEditor";
import {
  Badge,
  Button,
  Card,
  CardHeader,
  ConfirmDialog,
  Field,
  IconButton,
  Input,
  Mono,
  PageHeader,
  Segmented,
  Select,
  Skeleton,
  Switch,
  Textarea,
  buttonClass,
} from "../../../components/console/ui";

function parseParams(text: string): { params: Record<string, string>; error: string } {
  const params: Record<string, string> = {};
  const lines = text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean);
  for (const line of lines) {
    const idx = line.indexOf("=");
    if (idx <= 0) return { params, error: `“${line}” 格式不正确，每行应为 键=值` };
    params[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
  }
  return { params, error: "" };
}

type AppModalTab = "config" | "basic" | "versions" | "schema" | "commands" | "metrics" | "presets";
const VALID_APP_TABS: AppModalTab[] = [
  "config",
  "basic",
  "versions",
  "schema",
  "commands",
  "metrics",
  "presets",
];

function isValidAppTab(v: string | null): v is AppModalTab {
  return !!v && (VALID_APP_TABS as string[]).includes(v);
}

function AdminRuntimesInner() {
  const { apps, loading, apiFetch, toast, reload } = useConsole();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const hydratedAppRef = useRef<string | null>(null);
  const [urlRev, setUrlRev] = useState(0);

  useEffect(() => {
    const onPopState = () => setUrlRev((r) => r + 1);
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  function syncUrlParams(updates: Record<string, string | null>) {
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    for (const [k, v] of Object.entries(updates)) {
      if (v === null || v === undefined || v === "") sp.delete(k);
      else sp.set(k, v);
    }
    const qs = sp.toString();
    const nextUrl = qs ? `${pathname}?${qs}` : pathname;
    window.history.replaceState(window.history.state, "", nextUrl);
  }

  // --------------------------------------------------- App Marketplace state
  const [appCat, setAppCat] = useState<"all" | "sandbox" | "survival" | "custom">(() => {
    const c = searchParams.get("category");
    return c === "sandbox" || c === "survival" || c === "custom" ? c : "all";
  });
  const [appQuery, setAppQuery] = useState("");
  const [appModalOpen, setAppModalOpen] = useState(false);
  const [editingApp, setEditingApp] = useState<AppItem | null>(null);
  const [modalTab, setModalTab] = useState<AppModalTab>("config");

  const [appName, setAppName] = useState("");
  const [appDisplayName, setAppDisplayName] = useState("");
  const [appTagline, setAppTagline] = useState("");
  const [appSummary, setAppSummary] = useState("");
  const [appCategory, setAppCategory] = useState("survival");
  const [appCover, setAppCover] = useState("");
  const [appImage, setAppImage] = useState("");
  const [appImageTemplate, setAppImageTemplate] = useState("");
  const [appPreStartCommand, setAppPreStartCommand] = useState("");
  const [appStartupCommand, setAppStartupCommand] = useState("");
  const [appPreStopCommand, setAppPreStopCommand] = useState("");
  const [appVersionsText, setAppVersionsText] = useState("latest");
  const [appRecVersion, setAppRecVersion] = useState("latest");
  const [previewVersion, setPreviewVersion] = useState("latest");
  const [appServerBinding, setAppServerBinding] = useState<AppServerBinding | undefined>(undefined);
  const [appAddonBinding, setAppAddonBinding] = useState<AppAddonBinding | undefined>(undefined);
  const [appCpu, setAppCpu] = useState("2");
  const [appMemory, setAppMemory] = useState("4Gi");
  const [appStorage, setAppStorage] = useState("20Gi");
  const [appPorts, setAppPorts] = useState<InstancePort[]>([{ name: "game", protocol: "TCP", targetPort: 25565 }]);
  const [appEnvText, setAppEnvText] = useState("");
  const [appSchema, setAppSchema] = useState<AppConfigField[]>([]);
  const [appPresets, setAppPresets] = useState<AppConfigPreset[]>([]);
  const [appTemplates, setAppTemplates] = useState<AppFileTemplate[]>([]);
  const [appConsoleDelivery, setAppConsoleDelivery] = useState<string>("stdin");
  const [appConsoleTargets, setAppConsoleTargets] = useState<AppConsoleTarget[]>([]);
  const [appConsoleActions, setAppConsoleActions] = useState<AppConsoleAction[]>([]);
  const [cmdJsonOpen, setCmdJsonOpen] = useState<boolean>(false);
  const [editingCmdId, setEditingCmdId] = useState<string | null>(null);
  const [appMetrics, setAppMetrics] = useState<AppMetricSpec[]>([]);
  const [metricsJsonOpen, setMetricsJsonOpen] = useState<boolean>(false);
  const [editingMetricId, setEditingMetricId] = useState<string | null>(null);
  const [appStoragePolicy, setAppStoragePolicy] = useState<AppStoragePolicy | undefined>(undefined);
  const [appPublished, setAppPublished] = useState(true);
  const [initialAppSnapshot, setInitialAppSnapshot] = useState<string>("");

  const [schemaScopeFilter, setSchemaScopeFilter] = useState<string>("基础设置");
  const [schemaGroupFilter, setSchemaGroupFilter] = useState<string>("all");
  const [schemaSearch, setSchemaSearch] = useState<string>("");
  const [addFieldOpen, setAddFieldOpen] = useState<boolean>(false);
  const [jsonEditorOpen, setJsonEditorOpen] = useState(false);
  const [jsonEditorText, setJsonEditorText] = useState("");
  const [newField, setNewField] = useState<AppConfigField>({
    name: "",
    label: "",
    type: "text",
    group: "基础设置",
    default: "",
    envKey: "",
    fileTarget: "",
  });
  const [presetFormOpen, setPresetFormOpen] = useState(false);
  const [presetsSubView, setPresetsSubView] = useState<"playstyle" | "templates">("playstyle");
  const [editingPresetIdx, setEditingPresetIdx] = useState<number | null>(null);
  const [presetId, setPresetId] = useState("");
  const [presetName, setPresetName] = useState("");
  const [presetDesc, setPresetDesc] = useState("");
  const [presetValuesText, setPresetValuesText] = useState("");

  const [appBusy, setAppBusy] = useState(false);
  const [deleteAppTarget, setDeleteAppTarget] = useState<AppItem | null>(null);
  const [deletingApp, setDeletingApp] = useState(false);

  const enrichedApps = useMemo(() => {
    const base = apps || [];
    return base.map((a) => resolveRichApp(a.metadata.name, apps) || a);
  }, [apps]);

  const filteredApps = useMemo(() => {
    const q = appQuery.trim().toLowerCase();
    return enrichedApps.filter((a) => {
      if (appCat !== "all" && (a.category || "survival") !== appCat) return false;
      if (!q) return true;
      const hay = `${a.metadata.name} ${a.displayName} ${a.tagline || ""} ${a.summary || ""} ${a.image}`.toLowerCase();
      return hay.includes(q);
    });
  }, [enrichedApps, appCat, appQuery]);

  function splitSchemaGroup(rawGroup?: string): { scope: string; category: string; full: string } {
    const g = (rawGroup || "").trim() || "基础设置";
    const idx = g.indexOf(" / ");
    if (idx > 0) {
      return {
        scope: g.slice(0, idx).trim(),
        category: g.slice(idx + 3).trim(),
        full: g,
      };
    }
    return { scope: g, category: g, full: g };
  }

  const schemaScopes = useMemo(() => {
    const out: string[] = [];
    for (const f of appSchema) {
      const { scope } = splitSchemaGroup(f.group);
      if (!out.includes(scope)) out.push(scope);
    }
    return out;
  }, [appSchema]);

  const hasMultiScope = useMemo(
    () => appSchema.some((f) => (f.group || "").includes(" / ")),
    [appSchema],
  );

  const schemaCategoriesForScope = useMemo(() => {
    const out: Array<{ full: string; category: string; count: number }> = [];
    for (const f of appSchema) {
      const { scope, category, full } = splitSchemaGroup(f.group);
      if (hasMultiScope && scope !== schemaScopeFilter) continue;
      const existing = out.find((x) => x.full === full);
      if (existing) {
        existing.count += 1;
      } else {
        out.push({ full, category, count: 1 });
      }
    }
    return out;
  }, [appSchema, hasMultiScope, schemaScopeFilter]);

  function closeAppDetail() {
    setAppModalOpen(false);
    hydratedAppRef.current = "__closed__";
    syncUrlParams({ app: null, tab: null, scope: null });
  }

  function changeModalTab(nextTab: AppModalTab) {
    setModalTab(nextTab);
    syncUrlParams({
      app: editingApp ? editingApp.metadata.name : "__new__",
      tab: nextTab,
      scope: null,
    });
  }

  function openCreateApp() {
    setEditingApp(null);
    setModalTab("basic");
    setAppName("");
    setAppDisplayName("");
    setAppTagline("");
    setAppSummary("");
    setAppCategory("survival");
    setAppCover("");
    setAppImage("gamepanel/runtime:latest");
    setAppImageTemplate("");
    setAppPreStartCommand("");
    setAppStartupCommand("");
    setAppPreStopCommand("");
    setAppVersionsText("latest");
    setAppRecVersion("latest");
    setPreviewVersion("latest");
    setAppServerBinding(undefined);
    setAppAddonBinding(undefined);
    setAppCpu("2");
    setAppMemory("4Gi");
    setAppStorage("20Gi");
    setAppPorts([{ name: "game", protocol: "TCP", targetPort: 25565 }]);
    setAppEnvText("");
    setAppSchema([]);
    setAppPresets([]);
    setAppTemplates([]);
    setAppConsoleDelivery("stdin");
    setAppConsoleTargets([]);
    setAppConsoleActions([]);
    setCmdJsonOpen(false);
    setEditingCmdId(null);
    setAppMetrics([]);
    setMetricsJsonOpen(false);
    setEditingMetricId(null);
    setAppStoragePolicy(undefined);
    setAppPublished(true);
    setInitialAppSnapshot(
      JSON.stringify({
        displayName: "",
        tagline: "",
        summary: "",
        category: "survival",
        cover: "",
        image: "gamepanel/runtime:latest",
        imageTemplate: "",
        preStartCommand: "",
        startupCommand: "",
        preStopCommand: "",
        versionsText: "latest",
        recVersion: "latest",
        serverBinding: undefined,
        addonBinding: undefined,
        cpu: "2",
        memory: "4Gi",
        storage: "20Gi",
        ports: [{ name: "game", protocol: "TCP", targetPort: 25565 }],
        envText: "",
        schema: [],
        presets: [],
        templates: [],
        delivery: "stdin",
        targets: [],
        actions: [],
        metrics: [],
        storagePolicy: undefined,
        published: true,
      }),
    );
    setSchemaScopeFilter("基础设置");
    setSchemaGroupFilter("all");
    setSchemaSearch("");
    setAddFieldOpen(false);
    setJsonEditorOpen(false);
    setPresetsSubView("playstyle");
    setPresetFormOpen(false);
    setEditingPresetIdx(null);
    setAppModalOpen(true);
    hydratedAppRef.current = "__new__";
    syncUrlParams({ app: "__new__", tab: "basic", scope: null });
  }

  function openEditApp(app: AppItem, initialTab: AppModalTab = "config") {
    const initName = app.metadata.name;
    const initDisplayName = app.displayName || app.metadata.name;
    const initTagline = app.tagline || "";
    const initSummary = app.summary || "";
    const initCategory = app.category || "survival";
    const initCover = app.coverImage || "";
    const initImage = app.image || "";
    const initImageTemplate = app.imageTemplate || "";
    const initPreStartCommand = app.preStartCommand || "";
    const initStartupCommand = app.startupCommand || "";
    const initPreStopCommand = app.preStopCommand || "";
    const initVersionsText = (app.versions || ["latest"]).join(", ");
    const initRecVersion = app.recommendedVersion || app.versions?.[0] || "latest";
    const initServerBinding: AppServerBinding | undefined = app.serverBinding
      ? { ...app.serverBinding }
      : undefined;
    const initAddonBinding: AppAddonBinding | undefined = app.addonBinding
      ? { ...app.addonBinding }
      : undefined;
    const initCpu = app.resources?.cpu || "2";
    const initMemory = app.resources?.memory || "4Gi";
    const initStorage = app.resources?.storage || "20Gi";
    const initPorts =
      app.ports && app.ports.length > 0
        ? app.ports.map((p) => ({ ...p }))
        : [{ name: "game", protocol: "TCP", targetPort: 25565 }];
    const initEnvText = Object.entries(app.env || {})
      .map(([k, v]) => `${k}=${v}`)
      .join("\n");
    const nextSchema = (app.configSchema || []).map((f) => ({ ...f }));
    const initPresets = (app.configPresets || []).map((p) => ({ ...p, values: { ...p.values } }));
    const initTemplates = (app.fileTemplates || []).map((t) => ({ ...t }));
    const initDelivery = app.consoleDelivery || "stdin";
    const initTargets = JSON.parse(JSON.stringify(app.consoleTargets || []));
    const initActions = JSON.parse(JSON.stringify(app.consoleActions || []));
    const initMetrics = JSON.parse(JSON.stringify(app.metrics || []));
    const initStoragePolicy = app.storagePolicy ? { ...app.storagePolicy } : undefined;
    const initPublished = app.published !== false;
    const resolvedTab: AppModalTab =
      initialTab === "config" && nextSchema.length === 0 ? "basic" : initialTab;

    setEditingApp(app);
    setModalTab(resolvedTab);
    setAppName(initName);
    setAppDisplayName(initDisplayName);
    setAppTagline(initTagline);
    setAppSummary(initSummary);
    setAppCategory(initCategory);
    setAppCover(initCover);
    setAppImage(initImage);
    setAppImageTemplate(initImageTemplate);
    setAppPreStartCommand(initPreStartCommand);
    setAppStartupCommand(initStartupCommand);
    setAppPreStopCommand(initPreStopCommand);
    setAppVersionsText(initVersionsText);
    setAppRecVersion(initRecVersion);
    setPreviewVersion(initRecVersion);
    setAppServerBinding(initServerBinding);
    setAppAddonBinding(initAddonBinding);
    setAppCpu(initCpu);
    setAppMemory(initMemory);
    setAppStorage(initStorage);
    setAppPorts(initPorts);
    setAppEnvText(initEnvText);
    setAppSchema(nextSchema);
    setAppPresets(initPresets);
    setAppTemplates(initTemplates);
    setAppConsoleDelivery(initDelivery);
    setAppConsoleTargets(initTargets);
    setAppConsoleActions(initActions);
    setCmdJsonOpen(false);
    setEditingCmdId(null);
    setAppMetrics(initMetrics);
    setMetricsJsonOpen(false);
    setEditingMetricId(null);
    setAppStoragePolicy(initStoragePolicy);
    setAppPublished(initPublished);
    setInitialAppSnapshot(
      JSON.stringify({
        displayName: initDisplayName,
        tagline: initTagline,
        summary: initSummary,
        category: initCategory,
        cover: initCover,
        image: initImage,
        imageTemplate: initImageTemplate,
        preStartCommand: initPreStartCommand,
        startupCommand: initStartupCommand,
        preStopCommand: initPreStopCommand,
        versionsText: initVersionsText,
        recVersion: initRecVersion,
        serverBinding: initServerBinding,
        addonBinding: initAddonBinding,
        cpu: initCpu,
        memory: initMemory,
        storage: initStorage,
        ports: initPorts,
        envText: initEnvText,
        schema: nextSchema,
        presets: initPresets,
        templates: initTemplates,
        delivery: initDelivery,
        targets: initTargets,
        actions: initActions,
        metrics: initMetrics,
        storagePolicy: initStoragePolicy,
        published: initPublished,
      }),
    );
    const firstGroup = nextSchema[0]?.group?.trim() || "基础设置";
    const firstParts = splitSchemaGroup(firstGroup);
    setSchemaScopeFilter(firstParts.scope);
    setSchemaGroupFilter(nextSchema.length > 24 ? firstParts.full : "all");
    setSchemaSearch("");
    setAddFieldOpen(false);
    setJsonEditorOpen(false);
    setPresetsSubView("playstyle");
    setPresetFormOpen(false);
    setEditingPresetIdx(null);
    setAppModalOpen(true);
    hydratedAppRef.current = initName;
    syncUrlParams({ app: initName, tab: resolvedTab });
  }

  useEffect(() => {
    const liveParams =
      typeof window !== "undefined" ? new URLSearchParams(window.location.search) : searchParams;
    const urlApp = liveParams.get("app");
    const urlTabRaw = liveParams.get("tab");
    if (!urlApp) {
      if (hydratedAppRef.current && hydratedAppRef.current !== "__closed__") {
        setAppModalOpen(false);
        hydratedAppRef.current = null;
      }
      return;
    }
    if (urlApp === "__new__") {
      if (hydratedAppRef.current !== "__new__") {
        openCreateApp();
      }
      return;
    }
    // Wait for API apps to load before hydrating from URL.
    if (loading && (!apps || apps.length === 0)) return;
    const target = enrichedApps.find((a) => a.metadata.name === urlApp);
    if (!target) return;
    const validTab: AppModalTab = isValidAppTab(urlTabRaw)
      ? urlTabRaw
      : (target.configSchema?.length || 0) > 0
        ? "config"
        : "basic";
    if (hydratedAppRef.current !== urlApp) {
      openEditApp(target, validTab);
    } else {
      setEditingApp(target);
      if (isValidAppTab(urlTabRaw) && urlTabRaw !== modalTab) {
        setModalTab(urlTabRaw);
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams, enrichedApps, loading, apps, urlRev]);

  const parsedVersions = useMemo(() => {
    const list = appVersionsText
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);
    return list.length > 0 ? list : ["latest"];
  }, [appVersionsText]);

  function openCreatePreset(seedFromDefaults = false) {
    setEditingPresetIdx(null);
    setPresetId("");
    setPresetName("");
    setPresetDesc("");
    if (seedFromDefaults && appSchema.length > 0) {
      setPresetValuesText(
        appSchema
          .filter((f) => f.default !== undefined && f.default !== "")
          .map((f) => `${f.name}=${f.default}`)
          .join("\n"),
      );
    } else {
      setPresetValuesText("");
    }
    setPresetFormOpen(true);
  }

  function openEditPreset(idx: number) {
    const p = appPresets[idx];
    if (!p) return;
    setEditingPresetIdx(idx);
    setPresetId(p.id);
    setPresetName(p.name);
    setPresetDesc(p.description || "");
    setPresetValuesText(
      Object.entries(p.values || {})
        .map(([k, v]) => `${k}=${v}`)
        .join("\n"),
    );
    setPresetFormOpen(true);
  }

  function savePresetItem() {
    const id = presetId.trim();
    const name = presetName.trim();
    if (!id || !name) {
      toast("error", "请填写预设标识与预设名称");
      return;
    }
    if (editingPresetIdx === null && appPresets.some((p) => p.id === id)) {
      toast("error", `预设标识 ${id} 已存在`);
      return;
    }
    const { params: values, error } = parseParams(presetValuesText);
    if (error) {
      toast("error", error);
      return;
    }
    const nextPreset: AppConfigPreset = {
      id,
      name,
      description: presetDesc.trim() || undefined,
      values,
    };
    setAppPresets((prev) =>
      editingPresetIdx === null
        ? [...prev, nextPreset]
        : prev.map((x, i) => (i === editingPresetIdx ? nextPreset : x)),
    );
    setPresetFormOpen(false);
    setEditingPresetIdx(null);
  }

  const draftApp = useMemo<AppItem>(() => {
    const versions = appVersionsText
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);
    return {
      metadata: {
        name: appName || editingApp?.metadata.name || "custom-app",
      },
      displayName: appDisplayName || editingApp?.displayName || "新游戏应用",
      tagline: appTagline,
      summary: appSummary,
      category: appCategory,
      coverImage: appCover,
      image: appImage || "alpine:latest",
      imageTemplate: appImageTemplate,
      versions: versions.length ? versions : ["latest"],
      recommendedVersion: appRecVersion || versions[0] || "latest",
      serverBinding: appServerBinding,
      addonBinding: appAddonBinding,
      ports: appPorts,
      configSchema: appSchema,
      configPresets: appPresets,
      fileTemplates: appTemplates,
      published: appPublished,
    };
  }, [
    appName,
    editingApp,
    appDisplayName,
    appTagline,
    appSummary,
    appCategory,
    appCover,
    appImage,
    appImageTemplate,
    appVersionsText,
    appRecVersion,
    appServerBinding,
    appAddonBinding,
    appPorts,
    appSchema,
    appPresets,
    appTemplates,
    appPublished,
  ]);

  const draftDefaultValues = useMemo(() => {
    const out: Record<string, string> = {};
    for (const f of appSchema) {
      out[f.name] = f.default ?? "";
    }
    return out;
  }, [appSchema]);

  function handleDraftDefaultsChange(nextValues: Record<string, string>) {
    setAppSchema((prev) =>
      prev.map((f) =>
        nextValues[f.name] !== undefined && nextValues[f.name] !== f.default
          ? { ...f, default: nextValues[f.name] }
          : f,
      ),
    );
  }

  function addSchemaField() {
    const key = newField.name.trim();
    const label = newField.label.trim() || key;
    if (!key) {
      toast("error", "请填写参数字段标识 (Key)");
      return;
    }
    if (appSchema.some((f) => f.name === key)) {
      toast("error", `参数字段 ${key} 已存在`);
      return;
    }
    setAppSchema((prev) => [
      ...prev,
      {
        name: key,
        label,
        type: newField.type,
        group: newField.group?.trim() || "基础设置",
        default: newField.default || "",
        envKey: newField.envKey?.trim() || undefined,
        fileTarget: newField.fileTarget?.trim() || undefined,
      },
    ]);
    setNewField({
      name: "",
      label: "",
      type: "text",
      group: newField.group || "基础设置",
      default: "",
      envKey: "",
      fileTarget: "",
    });
  }

  function applySchemaJson() {
    try {
      const parsed = JSON.parse(jsonEditorText);
      const list = Array.isArray(parsed) ? parsed : parsed.configSchema;
      if (!Array.isArray(list)) {
        throw new Error("JSON 需为配置字段数组或包含 configSchema 数组");
      }
      setAppSchema(list);
      setJsonEditorOpen(false);
      toast("success", `已导入 ${list.length} 项字段定义`);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "JSON 格式解析失败");
    }
  }

  async function submitApp(e?: FormEvent) {
    e?.preventDefault();
    const targetName = (editingApp ? editingApp.metadata.name : appName).trim();
    if (!editingApp) {
      const err = validateName(targetName, "应用标识");
      if (err) {
        toast("error", err);
        setModalTab("basic");
        return;
      }
    }
    if (!appDisplayName.trim()) {
      toast("error", "请填写应用名称");
      setModalTab("basic");
      return;
    }
    const { params: envMap, error: envErr } = parseParams(appEnvText);
    if (envErr) {
      toast("error", envErr);
      setModalTab("basic");
      return;
    }
    const versions = appVersionsText
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);

    setAppBusy(true);
    try {
      const payload: AppItem = {
        ...(editingApp || {}),
        metadata: {
          ...(editingApp?.metadata || {}),
          name: targetName,
          labels: {
            ...(editingApp?.metadata?.labels || {}),
            "gamepanel.io/category": appCategory,
          },
        },
        displayName: appDisplayName.trim(),
        tagline: appTagline.trim() || undefined,
        summary: appSummary.trim() || undefined,
        category: appCategory,
        coverImage: appCover.trim() || undefined,
        image: appImage.trim() || "gamepanel/runtime:latest",
        imageTemplate: undefined,
        preStartCommand: appPreStartCommand.trim() || undefined,
        startupCommand: appStartupCommand.trim() || undefined,
        preStopCommand: appPreStopCommand.trim() || undefined,
        versions: versions.length ? versions : ["latest"],
        recommendedVersion: appRecVersion.trim() || versions[0] || "latest",
        serverBinding:
          appServerBinding?.repository?.trim() && appServerBinding?.package?.trim()
            ? {
                repository: appServerBinding.repository.trim(),
                package: appServerBinding.package.trim(),
                targetPath: appServerBinding.targetPath?.trim() || undefined,
              }
            : undefined,
        addonBinding:
          appAddonBinding?.repository?.trim() && appAddonBinding?.targetDir?.trim()
            ? {
                repository: appAddonBinding.repository.trim(),
                installMode: appAddonBinding.installMode || "flat_dir",
                targetDir: appAddonBinding.targetDir.trim(),
              }
            : undefined,
        resources: {
          cpu: appCpu.trim() || "2",
          memory: appMemory.trim() || "4Gi",
          storage: appStorage.trim() || "20Gi",
        },
        ports: appPorts.map((p, idx) => ({
          name: p.name?.trim() || `port-${idx + 1}`,
          protocol: (p.protocol || "TCP").toUpperCase(),
          targetPort: Number(p.targetPort) || 25565,
          publishedPort: 0,
        })),
        env: Object.keys(envMap).length ? envMap : undefined,
        configSchema: appSchema,
        configPresets: appPresets,
        fileTemplates: appTemplates,
        consoleDelivery: appConsoleDelivery,
        consoleTargets: appConsoleTargets.length ? appConsoleTargets : undefined,
        consoleActions: appConsoleActions,
        metrics: appMetrics,
        storagePolicy: appStoragePolicy,
        published: appPublished,
        builtIn: editingApp?.builtIn,
      };

      const res = await apiFetch(
        editingApp
          ? `/apis/core.gamepanel.io/v1/apps/${encodeURIComponent(targetName)}`
          : "/apis/core.gamepanel.io/v1/apps",
        {
          method: editingApp ? "PUT" : "POST",
          body: JSON.stringify({
            apiVersion: "core.gamepanel.io/v1",
            kind: "App",
            ...payload,
          }),
        },
      );
      if (!res.ok) throw new Error(await errorMessage(res, "保存游戏应用失败"));
      toast("success", `游戏应用「${appDisplayName.trim()}」已${editingApp ? "更新" : "上架"}`);
      setAppBusy(false);
      if (editingApp) {
        setInitialAppSnapshot(currentAppSnapshot);
      } else {
        closeAppDetail();
      }
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "保存游戏应用失败");
      setAppBusy(false);
    }
  }

  async function confirmDeleteApp() {
    if (!deleteAppTarget) return;
    setDeletingApp(true);
    try {
      const res = await apiFetch(
        `/apis/core.gamepanel.io/v1/apps/${encodeURIComponent(deleteAppTarget.metadata.name)}`,
        { method: "DELETE" },
      );
      if (!res.ok) throw new Error(await errorMessage(res, "删除应用失败"));
      toast("success", `应用 ${deleteAppTarget.displayName} 已删除`);
      setDeleteAppTarget(null);
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "删除应用失败");
    } finally {
      setDeletingApp(false);
    }
  }

  const visibleSchemaFields = useMemo(() => {
    const q = schemaSearch.trim().toLowerCase();
    return appSchema.filter((f) => {
      const { scope, full } = splitSchemaGroup(f.group);
      if (q) {
        const hay = `${f.label} ${f.name} ${f.group || ""} ${f.envKey || ""} ${f.fileTarget || ""}`.toLowerCase();
        return hay.includes(q);
      }
      if (hasMultiScope && scope !== schemaScopeFilter) return false;
      if (schemaGroupFilter && schemaGroupFilter !== "all") {
        return full === schemaGroupFilter;
      }
      return true;
    });
  }, [appSchema, schemaSearch, hasMultiScope, schemaScopeFilter, schemaGroupFilter]);

  const currentAppSnapshot = useMemo(
    () =>
      JSON.stringify({
        displayName: appDisplayName,
        tagline: appTagline,
        summary: appSummary,
        category: appCategory,
        cover: appCover,
        image: appImage,
        imageTemplate: appImageTemplate,
        preStartCommand: appPreStartCommand,
        startupCommand: appStartupCommand,
        preStopCommand: appPreStopCommand,
        versionsText: appVersionsText,
        recVersion: appRecVersion,
        serverBinding: appServerBinding,
        addonBinding: appAddonBinding,
        cpu: appCpu,
        memory: appMemory,
        storage: appStorage,
        ports: appPorts,
        envText: appEnvText,
        schema: appSchema,
        presets: appPresets,
        templates: appTemplates,
        delivery: appConsoleDelivery,
        targets: appConsoleTargets,
        actions: appConsoleActions,
        metrics: appMetrics,
        storagePolicy: appStoragePolicy,
        published: appPublished,
      }),
    [
      appDisplayName,
      appTagline,
      appSummary,
      appCategory,
      appCover,
      appImage,
      appImageTemplate,
      appPreStartCommand,
      appStartupCommand,
      appPreStopCommand,
      appVersionsText,
      appRecVersion,
      appServerBinding,
      appAddonBinding,
      appCpu,
      appMemory,
      appStorage,
      appPorts,
      appEnvText,
      appSchema,
      appPresets,
      appTemplates,
      appConsoleDelivery,
      appConsoleTargets,
      appConsoleActions,
      appMetrics,
      appStoragePolicy,
      appPublished,
    ],
  );

  const appDirty = !editingApp
    ? Boolean(appName.trim() || currentAppSnapshot !== initialAppSnapshot)
    : currentAppSnapshot !== initialAppSnapshot;

  async function persistAppVersions(
    nextVersions: string[],
    nextRec: string,
    nextBinding?: AppServerBinding,
  ) {
    if (!editingApp) return;
    const targetName = editingApp.metadata.name;
    const versionsList = nextVersions.length ? nextVersions : ["latest"];
    const recommended = nextRec || versionsList[0] || "latest";
    const body: Record<string, unknown> = {
      versions: versionsList,
      recommendedVersion: recommended,
    };
    if (nextBinding !== undefined) {
      body.serverBinding = nextBinding;
    }
    const res = await apiFetch(`/apis/core.gamepanel.io/v1/apps/${encodeURIComponent(targetName)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/merge-patch+json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      throw new Error(await errorMessage(res, "保存游戏版本失败"));
    }
    const updated = (await res.json()) as AppItem;
    const savedVersions = updated.versions?.length ? updated.versions : versionsList;
    const savedRec = updated.recommendedVersion || recommended;
    const savedVersionsText = savedVersions.join(", ");
    setEditingApp(updated);
    setAppVersionsText(savedVersionsText);
    setAppRecVersion(savedRec);
    if (savedRec) setPreviewVersion(savedRec);
    if (nextBinding !== undefined) {
      setAppServerBinding(updated.serverBinding);
    }
    setInitialAppSnapshot((prev) => {
      if (!prev) return prev;
      try {
        const parsed = JSON.parse(prev) as Record<string, unknown>;
        parsed.versionsText = savedVersionsText;
        parsed.recVersion = savedRec;
        if (nextBinding !== undefined) {
          parsed.serverBinding = updated.serverBinding;
        }
        return JSON.stringify(parsed);
      } catch {
        return prev;
      }
    });
    await reload();
  }

  return (
    <div>
      {appModalOpen ? (
        <PageHeader
          title={editingApp ? appDisplayName || editingApp.displayName : "上架应用"}
          description={appTagline || appSummary || editingApp?.tagline || undefined}
          actions={
            <div className="flex flex-wrap items-center gap-2.5">
              <Button
                variant="secondary"
                icon={<ArrowLeft className="h-4 w-4" />}
                disabled={appBusy}
                onClick={closeAppDetail}
              >
                返回
              </Button>
              <Button
                variant="primary"
                loading={appBusy}
                disabled={appBusy || !appDirty}
                onClick={() => void submitApp()}
              >
                {editingApp ? "保存" : "确认上架"}
              </Button>
            </div>
          }
        />
      ) : (
        <PageHeader
          title="应用市场"
          description={
            apps.length > 0
              ? `已上架 ${apps.filter((a) => a.published !== false).length} 款 · 共 ${apps.length} 款应用`
              : undefined
          }
          actions={
            <Button variant="primary" icon={<Plus className="h-4 w-4" />} onClick={openCreateApp}>
              上架应用
            </Button>
          }
        />
      )}

      {!appModalOpen ? (
        <div className="space-y-5">
          {/* Single-row Filter Bar */}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Segmented
              label="应用分类筛选"
              value={appCat}
              onChange={(v) => {
                const next = v as "all" | "sandbox" | "survival" | "custom";
                setAppCat(next);
                syncUrlParams({ category: next === "all" ? null : next });
              }}
              items={[
                { value: "all", label: "全部" },
                { value: "sandbox", label: "沙盒建造" },
                { value: "survival", label: "生存合作" },
                { value: "custom", label: "自定义" },
              ]}
            />
            <div className="relative w-72">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[#8A90A6]" />
              <Input
                placeholder="搜索游戏名称、镜像或标识..."
                value={appQuery}
                onChange={(e) => setAppQuery(e.target.value)}
                className="pl-9"
              />
            </div>
          </div>

          {/* 3-Column Game Card Grid */}
          <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
            {filteredApps.map((app) => {
              const firstPort = app.ports?.[0];
              const schemaCount = app.configSchema?.length || 0;
              const res = app.resources || {};
              return (
                <Card key={app.metadata.name} className="overflow-hidden flex flex-col">
                  <div
                    role="button"
                    tabIndex={0}
                    onClick={() => openEditApp(app, schemaCount > 0 ? "config" : "basic")}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        openEditApp(app, schemaCount > 0 ? "config" : "basic");
                      }
                    }}
                    className="relative h-36 w-full cursor-pointer overflow-hidden bg-[#14161F]"
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={gameCover(app.metadata.name, app.coverImage)}
                      onError={(e) => {
                        if (!e.currentTarget.src.endsWith(DEFAULT_GAME_COVER)) {
                          e.currentTarget.src = DEFAULT_GAME_COVER;
                        }
                      }}
                      alt=""
                      className="h-full w-full object-cover transition duration-300 hover:scale-[1.02]"
                    />
                    <div className="absolute inset-0 bg-gradient-to-t from-[#14161F]/65 via-transparent to-transparent" />
                    <div className="absolute left-4 top-3.5">
                      <span className="rounded-full bg-black/50 px-2.5 py-0.5 text-[11px] font-medium text-white backdrop-blur-xs">
                        {formatAppCategory(app.category)}
                      </span>
                    </div>
                    <div className="absolute right-4 top-3.5">
                      <Badge tone={app.published !== false ? "success" : "warning"}>
                        {app.published !== false ? "已上架" : "已下架"}
                      </Badge>
                    </div>
                    <div className="absolute bottom-3 left-4 right-4 flex items-end justify-between gap-2">
                      <div className="min-w-0">
                        <div className="truncate text-base font-bold text-white">{app.displayName}</div>
                        <div className="truncate text-xs text-white/80">
                          {app.tagline || app.metadata.name}
                        </div>
                      </div>
                    </div>
                  </div>

                  <div className="flex flex-1 flex-col justify-between p-5">
                    <div>
                      <p className="line-clamp-2 text-[13px] leading-relaxed text-[#5B6075]">
                        {app.summary || app.tagline || "提供标准化的运行参数与配置模板。"}
                      </p>

                      <div className="mt-3.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[#5B6075]">
                        <Mono className="font-medium text-[#14161F]">
                          {app.recommendedVersion || app.versions?.[0] || "latest"}
                        </Mono>
                        <span className="text-[#CBD0E0]">·</span>
                        <span>
                          推荐 {res.cpu || "2"} 核 {String(res.memory || "4Gi").replace("Gi", " GB")}
                        </span>
                        {firstPort && (
                          <>
                            <span className="text-[#CBD0E0]">·</span>
                            <Mono>
                              {firstPort.targetPort}/{(firstPort.protocol || "TCP").toUpperCase()}
                              {(app.ports?.length || 0) > 1 ? ` +${(app.ports?.length || 1) - 1}` : ""}
                            </Mono>
                          </>
                        )}
                        {schemaCount > 0 && (
                          <>
                            <span className="text-[#CBD0E0]">·</span>
                            <span>{schemaCount} 项参数</span>
                          </>
                        )}
                      </div>
                    </div>

                    <div className="mt-4 flex items-center justify-between border-t border-[#EEF0F7] pt-3.5">
                      <Mono className="truncate text-[12px] text-[#8A90A6]">{app.image}</Mono>
                      <div className="flex shrink-0 items-center gap-1">
                        <IconButton
                          label={`编辑应用 ${app.displayName}`}
                          onClick={() => openEditApp(app, schemaCount > 0 ? "config" : "basic")}
                        >
                          <Pencil className="h-4 w-4" />
                        </IconButton>
                        {!app.builtIn && (
                          <IconButton
                            label={`删除应用 ${app.displayName}`}
                            onClick={() => setDeleteAppTarget(app)}
                            className="hover:!bg-[#FEF3F2] hover:!text-[#D92D20]"
                          >
                            <Trash2 className="h-4 w-4" />
                          </IconButton>
                        )}
                      </div>
                    </div>
                  </div>
                </Card>
              );
            })}

            {/* Dashed Add Card */}
            <button
              type="button"
              onClick={openCreateApp}
              className="flex min-h-[280px] flex-col items-center justify-center rounded-[20px] border-2 border-dashed border-[#D5DAE8] bg-white/50 p-6 text-center transition hover:border-[#2B3AE8] hover:bg-white"
            >
              <span className="flex h-12 w-12 items-center justify-center rounded-full bg-[#EEF1FF] text-[#2B3AE8]">
                <Plus className="h-5 w-5" />
              </span>
              <span className="mt-3 text-sm font-semibold text-[#14161F]">上架应用</span>
            </button>
          </div>
        </div>
      ) : (
        /* Full-Page Detail Editor Mode */
        <Card className="overflow-hidden">
          <CardHeader
            title={
              <Segmented
                label="应用配置维度"
                value={modalTab}
                onChange={(v) => changeModalTab(v as AppModalTab)}
                items={[
                  ...(appSchema.length > 0
                    ? [{ value: "config" as const, label: "参数预览" }]
                    : []),
                  { value: "basic" as const, label: "基础信息" },
                  { value: "versions" as const, label: "游戏版本" },
                  { value: "schema" as const, label: "字段定义" },
                  { value: "commands" as const, label: "游戏指令" },
                  { value: "metrics" as const, label: "监控指标" },
                  {
                    value: "presets" as const,
                    label: "预设模板",
                  },
                ]}
              />
            }
            actions={
              modalTab === "schema" ? (
                <div className="flex items-center gap-2">
                  {!jsonEditorOpen && (
                    <Button
                      size="sm"
                      variant={addFieldOpen ? "primary" : "secondary"}
                      icon={<Plus className="h-3.5 w-3.5" />}
                      onClick={() => {
                        setNewField((prev) => ({
                          ...prev,
                          group:
                            schemaGroupFilter && schemaGroupFilter !== "all"
                              ? schemaGroupFilter
                              : prev.group || "基础设置",
                        }));
                        setAddFieldOpen((v) => !v);
                      }}
                    >
                      {addFieldOpen ? "收起" : "新增字段"}
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="ghost"
                    icon={<Download className="h-3.5 w-3.5" />}
                    onClick={() => {
                      setJsonEditorText(JSON.stringify(appSchema, null, 2));
                      setJsonEditorOpen((v) => !v);
                    }}
                  >
                    {jsonEditorOpen ? "列表视图" : "JSON"}
                  </Button>
                </div>
              ) : modalTab === "commands" ? (
                <div className="flex flex-wrap items-center gap-2">
                  <div className="inline-flex items-center gap-2">
                    <label htmlFor="app-console-delivery" className="text-xs font-medium text-[#5B6075]">
                      投递通道
                    </label>
                    <Select
                      id="app-console-delivery"
                      value={appConsoleDelivery}
                      onChange={(e) => setAppConsoleDelivery(e.target.value)}
                      className="h-8 w-auto text-xs font-semibold"
                    >
                      <option value="stdin">标准输入通道</option>
                      <option value="rcon-cli">内置 RCON 协议</option>
                      <option value="palworld-send-to-console">幻兽帕鲁 REST/RCON 通道</option>
                      <option value="mc-send-to-console">Minecraft 控制台通道</option>
                      <option value="tml-send-to-console">tModLoader 控制台通道</option>
                      <option value="dst-send-to-console">饥荒多世界控制台通道</option>
                      <option value="7dtd-send-to-console">七日杀本地 Telnet 通道</option>
                    </Select>
                  </div>
                  <Segmented
                    size="sm"
                    label="指令编辑模式"
                    value={cmdJsonOpen ? "json" : "form"}
                    onChange={(v) => setCmdJsonOpen(v === "json")}
                    items={[
                      { value: "form", label: "表单" },
                      { value: "json", label: "JSON" },
                    ]}
                  />
                  <Button
                    size="sm"
                    variant="primary"
                    icon={<Plus className="h-3.5 w-3.5" />}
                    onClick={() => {
                      setCmdJsonOpen(false);
                      const newId = `cmd-${Date.now()}`;
                      const defaultGroup = appConsoleActions[0]?.group || "常用指令";
                      const newAction: AppConsoleAction = {
                        id: newId,
                        title: "新游戏指令",
                        group: defaultGroup,
                        command: 'say "{{msg}}"',
                        starred: true,
                        params: [
                          {
                            name: "msg",
                            label: "公告内容",
                            type: "text",
                            defaultValue: "服务器将在 5 分钟后重启维护",
                          },
                        ],
                      };
                      setAppConsoleActions((prev) => [newAction, ...prev]);
                      setEditingCmdId(newId);
                    }}
                  >
                    新增指令
                  </Button>
                </div>
              ) : modalTab === "metrics" ? (
                <div className="flex flex-wrap items-center gap-2">
                  <Segmented
                    size="sm"
                    label="指标编辑模式"
                    value={metricsJsonOpen ? "json" : "form"}
                    onChange={(v) => setMetricsJsonOpen(v === "json")}
                    items={[
                      { value: "form", label: "表单" },
                      { value: "json", label: "JSON" },
                    ]}
                  />
                  <Button
                    size="sm"
                    variant="primary"
                    icon={<Plus className="h-3.5 w-3.5" />}
                    onClick={() => {
                      setMetricsJsonOpen(false);
                      const newId = `metric_${appMetrics.length + 1}`;
                      const newMetric: AppMetricSpec = {
                        id: newId,
                        label: "新自定义监控指标",
                        unit: "",
                        source: "stdin_log",
                        mode: "regex_value",
                        probeCommand: "list",
                        pattern: "(\\d+)",
                      };
                      setAppMetrics((prev) => [newMetric, ...prev]);
                      setEditingMetricId(newId);
                    }}
                  >
                    新增指标
                  </Button>
                </div>
              ) : (
                <div className="flex items-center gap-2">
                  {appName && <Mono className="text-xs text-[#8A90A6]">{appName}</Mono>}
                  <Badge tone={appPublished ? "success" : "warning"}>
                    {appPublished ? "已上架" : "已下架"}
                  </Badge>
                </div>
              )
            }
          />

          {(modalTab === "config" || modalTab === "basic") && (
            <div className="px-6 py-5">
              {modalTab === "config" && appSchema.length > 0 && (
                <GameConfigEditor
                  app={draftApp}
                  values={draftDefaultValues}
                  onChange={handleDraftDefaultsChange}
                  version={previewVersion}
                  onVersionChange={setPreviewVersion}
                  syncUrlParams={false}
                />
              )}

              {modalTab === "basic" && (
                <div className="space-y-6">
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
                    <Field label="应用名称" htmlFor="app-display">
                      <Input
                        id="app-display"
                        placeholder="例如 幻兽帕鲁 Palworld"
                        value={appDisplayName}
                        onChange={(e) => setAppDisplayName(e.target.value)}
                      />
                    </Field>
                    <Field label="应用标识" htmlFor="app-id">
                      <Input
                        id="app-id"
                        disabled={!!editingApp}
                        placeholder="palworld-dedicated"
                        value={appName}
                        className="font-mono"
                        onChange={(e) => setAppName(e.target.value.toLowerCase())}
                      />
                    </Field>
                    <Field label="分类" htmlFor="app-cat">
                      <Select id="app-cat" value={appCategory} onChange={(e) => setAppCategory(e.target.value)}>
                        <option value="sandbox">沙盒建造</option>
                        <option value="survival">生存合作</option>
                        <option value="custom">自定义</option>
                      </Select>
                    </Field>
                    <Field label="副标题" htmlFor="app-tagline">
                      <Input
                        id="app-tagline"
                        placeholder="开放世界生存制作 · 32 人联机服"
                        value={appTagline}
                        onChange={(e) => setAppTagline(e.target.value)}
                      />
                    </Field>
                  </div>

                  <Field label="简介" htmlFor="app-summary">
                    <Input
                      id="app-summary"
                      placeholder="简要说明支持的游戏特性与配置能力"
                      value={appSummary}
                      onChange={(e) => setAppSummary(e.target.value)}
                    />
                  </Field>

                  {/* Game Cover Art Upload & URL Editor */}
                  <div className="flex flex-col gap-4 border-t border-[#EEF0F7] pt-5 sm:flex-row sm:items-center">
                    <div className="relative h-24 w-40 shrink-0 overflow-hidden rounded-xl bg-[#14161F] ring-1 ring-[#14161F]/10">
                      {/* eslint-disable-next-line @next/next/no-img-element */}
                      <img
                        src={gameCover(appName || "custom-container", appCover)}
                        onError={(e) => {
                          if (!e.currentTarget.src.endsWith(DEFAULT_GAME_COVER)) {
                            e.currentTarget.src = DEFAULT_GAME_COVER;
                          }
                        }}
                        alt=""
                        className="h-full w-full object-cover"
                      />
                    </div>
                    <div className="min-w-0 flex-1 space-y-2">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="text-[13px] font-medium text-[#3D4258]">游戏封面图</span>
                        <div className="flex items-center gap-2">
                          <label className={buttonClass("secondary", "sm", "cursor-pointer")}>
                            <ImagePlus className="h-3.5 w-3.5" />
                            <span>上传封面</span>
                            <input
                              type="file"
                              accept="image/*"
                              className="hidden"
                              onChange={async (e) => {
                                const file = e.target.files?.[0];
                                e.target.value = "";
                                if (!file) return;
                                try {
                                  const dataUrl = await compressCoverImageFile(file);
                                  setAppCover(dataUrl);
                                  toast("success", "已载入封面图片，点击右上角保存生效");
                                } catch (err) {
                                  toast("error", err instanceof Error ? err.message : "图片处理失败");
                                }
                              }}
                            />
                          </label>
                          {appCover && appCover !== defaultGameCover(appName) && (
                            <Button
                              type="button"
                              size="sm"
                              variant="ghost"
                              icon={<RotateCcw className="h-3.5 w-3.5" />}
                              onClick={() => setAppCover(defaultGameCover(appName))}
                            >
                              恢复默认封面
                            </Button>
                          )}
                        </div>
                      </div>
                      <Input
                        id="app-cover"
                        placeholder={defaultGameCover(appName || "custom-container")}
                        value={appCover.startsWith("data:image/") ? "" : appCover}
                        onChange={(e) => setAppCover(e.target.value)}
                        className="font-mono text-xs"
                      />
                      {appCover.startsWith("data:image/") && (
                        <div className="text-[11px] text-[#2B3AE8]">已使用本地上传封面</div>
                      )}
                    </div>
                  </div>

                  <div className="space-y-4 border-t border-[#EEF0F7] pt-5">
                    <Field label="启动前脚本 (选填)" htmlFor="app-pre-start-cmd">
                      <Textarea
                        id="app-pre-start-cmd"
                        rows={3}
                        className="font-mono text-xs"
                        placeholder="chmod +x ./start-server.sh"
                        value={appPreStartCommand}
                        onChange={(e) => setAppPreStartCommand(e.target.value)}
                      />
                    </Field>

                    <Field label="启动命令" htmlFor="app-startup-cmd">
                      <Input
                        id="app-startup-cmd"
                        className="font-mono text-xs"
                        placeholder="例如 java -Xms${MEMORY:-2G} -Xmx${MEMORY:-2G} -jar paper-{{Version}}.jar --nogui"
                        value={appStartupCommand}
                        onChange={(e) => setAppStartupCommand(e.target.value)}
                      />
                    </Field>

                    <Field label="停止前优雅存盘脚本 (选填)" htmlFor="app-pre-stop-cmd">
                      <Textarea
                        id="app-pre-stop-cmd"
                        rows={3}
                        className="font-mono text-xs"
                        placeholder="容器停止/重启/重建前执行，例如保存存档并优雅退出游戏进程"
                        value={appPreStopCommand}
                        onChange={(e) => setAppPreStopCommand(e.target.value)}
                      />
                    </Field>

                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                      <Field label="CPU (核)" htmlFor="app-cpu">
                        <Input id="app-cpu" className="font-mono" value={appCpu} onChange={(e) => setAppCpu(e.target.value)} />
                      </Field>
                      <Field label="内存" htmlFor="app-mem">
                        <Input
                          id="app-mem"
                          className="font-mono"
                          value={appMemory}
                          onChange={(e) => setAppMemory(e.target.value)}
                        />
                      </Field>
                      <Field label="存储" htmlFor="app-disk">
                        <Input
                          id="app-disk"
                          className="font-mono"
                          value={appStorage}
                          onChange={(e) => setAppStorage(e.target.value)}
                        />
                      </Field>
                    </div>
                  </div>

                  {/* Multi-Port Editor */}
                  <div className="space-y-3 border-t border-[#EEF0F7] pt-5">
                    <div className="flex items-center justify-between">
                      <span className="text-[13px] font-medium text-[#3D4258]">端口映射</span>
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        icon={<Plus className="h-3.5 w-3.5" />}
                        onClick={() =>
                          setAppPorts((prev) => [
                            ...prev,
                            { name: `port-${prev.length + 1}`, protocol: "UDP", targetPort: 27015 },
                          ])
                        }
                      >
                        添加端口
                      </Button>
                    </div>
                    <div className="space-y-2.5">
                      {appPorts.map((p, idx) => (
                        <div key={idx} className="grid grid-cols-[1fr_140px_160px_auto] items-center gap-3">
                          <Input
                            placeholder="端口名 (如 game)"
                            value={p.name || ""}
                            className="font-mono"
                            onChange={(e) =>
                              setAppPorts((prev) => prev.map((x, i) => (i === idx ? { ...x, name: e.target.value } : x)))
                            }
                          />
                          <Select
                            value={(p.protocol || "TCP").toUpperCase()}
                            onChange={(e) =>
                              setAppPorts((prev) =>
                                prev.map((x, i) => (i === idx ? { ...x, protocol: e.target.value } : x)),
                              )
                            }
                          >
                            <option value="TCP">TCP</option>
                            <option value="UDP">UDP</option>
                          </Select>
                          <Input
                            type="number"
                            placeholder="容器端口"
                            value={String(p.targetPort || "")}
                            className="font-mono"
                            onChange={(e) =>
                              setAppPorts((prev) =>
                                prev.map((x, i) => (i === idx ? { ...x, targetPort: Number(e.target.value) } : x)),
                              )
                            }
                          />
                          {appPorts.length > 1 && (
                            <IconButton
                              label="删除端口"
                              onClick={() => setAppPorts((prev) => prev.filter((_, i) => i !== idx))}
                            >
                              <Trash2 className="h-4 w-4" />
                            </IconButton>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="space-y-4 border-t border-[#EEF0F7] pt-5">
                    <Field label="环境变量" htmlFor="app-env">
                      <Textarea
                        id="app-env"
                        rows={4}
                        className="font-mono"
                        placeholder="EULA=TRUE&#10;TYPE=PAPER"
                        value={appEnvText}
                        onChange={(e) => setAppEnvText(e.target.value)}
                      />
                    </Field>

                    <div className="inline-flex items-center gap-2.5">
                      <Switch id="app-published" checked={appPublished} onChange={setAppPublished} />
                      <label htmlFor="app-published" className="cursor-pointer select-none text-sm text-[#14161F]">
                        公开上架
                      </label>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {modalTab === "versions" && (
            <GameVersionsPanel
              appName={appName || editingApp?.metadata.name || ""}
              appDisplayName={appDisplayName || editingApp?.displayName || ""}
              versions={parsedVersions}
              recommendedVersion={appRecVersion}
              serverBinding={appServerBinding}
              onVersionsChange={(nextVersions, nextRec) => {
                setAppVersionsText(nextVersions.join(", "));
                setAppRecVersion(nextRec);
                if (nextRec) setPreviewVersion(nextRec);
              }}
              onServerBindingChange={setAppServerBinding}
              onPersistVersions={editingApp ? persistAppVersions : undefined}
            />
          )}

          {modalTab === "schema" && (
            <div>
              {jsonEditorOpen ? (
                <div className="space-y-3 px-6 py-5">
                  <Field label="字段定义 JSON" htmlFor="schema-json">
                    <Textarea
                      id="schema-json"
                      rows={16}
                      className="max-h-[480px] font-mono text-xs"
                      value={jsonEditorText}
                      onChange={(e) => setJsonEditorText(e.target.value)}
                    />
                  </Field>
                  <div className="flex justify-end gap-2">
                    <Button size="sm" variant="ghost" onClick={() => setJsonEditorOpen(false)}>
                      取消
                    </Button>
                    <Button size="sm" variant="primary" onClick={applySchemaJson}>
                      应用 JSON
                    </Button>
                  </div>
                </div>
              ) : (
                <>
                  {/* Single-row Filter Bar */}
                  <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[#EEF0F7] px-6 py-3.5">
                    <div className="flex flex-wrap items-center gap-2.5">
                      {hasMultiScope && (
                        <Segmented
                          size="sm"
                          label="配置分组维度"
                          value={schemaSearch.trim() ? "" : schemaScopeFilter}
                          onChange={(sc) => {
                            setSchemaScopeFilter(sc);
                            setSchemaSearch("");
                            const firstInScope = appSchema.find((f) => splitSchemaGroup(f.group).scope === sc);
                            setSchemaGroupFilter(
                              appSchema.length > 24 && firstInScope
                                ? splitSchemaGroup(firstInScope.group).full
                                : "all",
                            );
                          }}
                          items={schemaScopes.map((sc) => ({ value: sc, label: sc }))}
                        />
                      )}

                      {schemaCategoriesForScope.length > 1 && (
                        <Select
                          aria-label="字段分类筛选"
                          value={schemaSearch.trim() ? "all" : schemaGroupFilter}
                          onChange={(e) => {
                            setSchemaSearch("");
                            setSchemaGroupFilter(e.target.value);
                          }}
                          className="h-8 w-auto text-xs"
                        >
                          <option value="all">全部分类</option>
                          {schemaCategoriesForScope.map((cat) => (
                            <option key={cat.full} value={cat.full}>
                              {cat.category}
                            </option>
                          ))}
                        </Select>
                      )}

                      <span className="text-xs text-[#8A90A6]">
                        共 {visibleSchemaFields.length} 项字段
                      </span>

                      {(visibleSchemaFields[0]?.fileTarget || appTemplates[0]?.path) && (
                        <span
                          className="inline-flex min-w-0 items-center gap-1 text-xs text-[#5B6075]"
                          title={visibleSchemaFields[0]?.fileTarget || appTemplates[0]?.path}
                        >
                          <FileCode2 className="h-3.5 w-3.5 shrink-0 text-[#2B3AE8]" />
                          <Mono className="max-w-[260px] truncate text-xs">
                            {(visibleSchemaFields[0]?.fileTarget || appTemplates[0]?.path || "")
                              .split("/")
                              .slice(-2)
                              .join("/")}
                          </Mono>
                        </span>
                      )}
                    </div>

                    <div className="relative w-60">
                      <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8A90A6]" />
                      <Input
                        placeholder="搜索字段..."
                        value={schemaSearch}
                        onChange={(e) => setSchemaSearch(e.target.value)}
                        className="h-9 pl-8 text-xs"
                      />
                    </div>
                  </div>

                  {/* On-Demand Collapsible Add Field Band */}
                  {addFieldOpen && (
                    <div className="border-b border-[#EEF0F7] bg-[#F7F8FC] px-6 py-4">
                      <div className="mb-2.5 text-xs font-semibold text-[#14161F]">新增字段</div>
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-12">
                        <div className="sm:col-span-3">
                          <Input
                            placeholder="字段 Key (如 expRate)"
                            value={newField.name}
                            className="h-9 font-mono text-xs"
                            onChange={(e) => setNewField((prev) => ({ ...prev, name: e.target.value }))}
                          />
                        </div>
                        <div className="sm:col-span-3">
                          <Input
                            placeholder="名称 (如 经验倍率)"
                            value={newField.label}
                            className="h-9 text-xs"
                            onChange={(e) => setNewField((prev) => ({ ...prev, label: e.target.value }))}
                          />
                        </div>
                        <div className="sm:col-span-2">
                          <Input
                            placeholder="分组"
                            value={newField.group || ""}
                            className="h-9 text-xs"
                            onChange={(e) => setNewField((prev) => ({ ...prev, group: e.target.value }))}
                          />
                        </div>
                        <div className="sm:col-span-2">
                          <Select
                            value={newField.type}
                            className="h-9 text-xs"
                            onChange={(e) =>
                              setNewField((prev) => ({
                                ...prev,
                                type: e.target.value as AppConfigField["type"],
                              }))
                            }
                          >
                            <option value="text">文本</option>
                            <option value="number">数值</option>
                            <option value="select">下拉</option>
                            <option value="boolean">开关</option>
                            <option value="password">密码</option>
                          </Select>
                        </div>
                        <div className="flex items-center gap-2 sm:col-span-2">
                          <Input
                            placeholder="默认值"
                            value={newField.default || ""}
                            className="h-9 font-mono text-xs"
                            onChange={(e) => setNewField((prev) => ({ ...prev, default: e.target.value }))}
                          />
                          <Button type="button" size="sm" variant="primary" className="shrink-0" onClick={addSchemaField}>
                            添加
                          </Button>
                        </div>
                      </div>
                    </div>
                  )}

                  {/* Full-Bleed 5-Column Schema Table */}
                  <div className="overflow-x-auto" tabIndex={0} role="region" aria-label="字段定义列表">
                    <table className="w-full table-fixed text-left text-sm">
                      <thead className="border-b border-[#EEF0F7] bg-[#FAFBFD] text-xs text-[#8A90A6]">
                        <tr>
                          <th className="w-[38%] px-6 py-3 font-medium">字段名称</th>
                          <th className="w-[12%] px-6 py-3 font-medium">控件类型</th>
                          <th className="w-[28%] px-6 py-3 font-medium">范围与映射</th>
                          <th className="w-[14%] px-6 py-3 text-right font-medium">默认值</th>
                          <th className="w-[8%] px-6 py-3 text-right font-medium">操作</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-[#EEF0F7] text-sm">
                        {visibleSchemaFields.length === 0 ? (
                          <tr>
                            <td colSpan={5} className="px-6 py-12 text-center text-sm text-[#8A90A6]">
                              暂无匹配字段
                            </td>
                          </tr>
                        ) : (
                          visibleSchemaFields.map((f) => {
                            const typeLabel =
                              f.type === "number" && f.min !== undefined
                                ? "滑块"
                                : f.type === "number"
                                  ? "数值"
                                  : f.type === "select"
                                    ? "下拉"
                                    : f.type === "boolean"
                                      ? "开关"
                                      : f.type === "password"
                                        ? "密码"
                                        : "文本";
                            const bounds =
                              f.type === "number" && f.min !== undefined && f.max !== undefined
                                ? `${f.min} ~ ${f.max}`
                                : f.options && f.options.length > 0
                                  ? `${f.options
                                      .slice(0, 4)
                                      .map((o) => o.label)
                                      .join(" / ")}${f.options.length > 4 ? ` 等 ${f.options.length} 项` : ""}`
                                  : f.type === "boolean"
                                    ? "True / False"
                                    : f.envKey || "-";
                            return (
                              <tr key={f.name} className="transition-colors hover:bg-[#FAFBFD]">
                                <td className="px-6 py-4">
                                  <div className="truncate font-medium text-[#14161F]" title={`${f.label} (${f.name})`}>
                                    {f.label}
                                  </div>
                                  <Mono className="mt-0.5 block truncate text-xs text-[#8A90A6]">{f.name}</Mono>
                                </td>
                                <td className="whitespace-nowrap px-6 py-4">
                                  <Badge tone="neutral">{typeLabel}</Badge>
                                </td>
                                <td
                                  className="truncate px-6 py-4 text-xs text-[#5B6075]"
                                  title={
                                    f.options && f.options.length > 0
                                      ? f.options.map((o) => `${o.label}(${o.value})`).join(" / ")
                                      : bounds
                                  }
                                >
                                  {bounds}
                                </td>
                                <td
                                  className="whitespace-nowrap px-6 py-4 text-right"
                                  title={String(f.default ?? "")}
                                >
                                  <Mono className="inline-block max-w-[120px] truncate align-bottom text-xs text-[#14161F]">
                                    {f.default !== undefined && f.default !== "" ? f.default : "-"}
                                  </Mono>
                                </td>
                                <td className="whitespace-nowrap px-6 py-4 text-right">
                                  <IconButton
                                    label={`删除字段 ${f.name}`}
                                    onClick={() => setAppSchema((prev) => prev.filter((x) => x.name !== f.name))}
                                    className="hover:!bg-[#FEF3F2] hover:!text-[#D92D20]"
                                  >
                                    <Trash2 className="h-3.5 w-3.5" aria-hidden />
                                  </IconButton>
                                </td>
                              </tr>
                            );
                          })
                        )}
                      </tbody>
                    </table>
                  </div>
                </>
              )}
            </div>
          )}

          {modalTab === "presets" && (
            <div>
              {/* Mutually Exclusive Sub-View Bar */}
              <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[#EEF0F7] px-6 py-3.5">
                <div className="flex flex-wrap items-center gap-3">
                  <Segmented
                    size="sm"
                    label="预设与模板切换"
                    value={presetsSubView}
                    onChange={(v) => setPresetsSubView(v as "playstyle" | "templates")}
                    items={[
                      { value: "playstyle", label: "玩法预设" },
                      { value: "templates", label: "配置文件模板" },
                    ]}
                  />
                  <span className="text-xs text-[#8A90A6]">
                    {presetsSubView === "playstyle"
                      ? `共 ${appPresets.length} 套玩法预设`
                      : `共 ${appTemplates.length} 份文件模板`}
                  </span>
                </div>

                {presetsSubView === "playstyle" ? (
                  <div className="flex items-center gap-2">
                    {appSchema.length > 0 && !presetFormOpen && (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => openCreatePreset(true)}
                      >
                        基于默认参数创建
                      </Button>
                    )}
                    <Button
                      type="button"
                      size="sm"
                      variant={presetFormOpen ? "primary" : "secondary"}
                      icon={<Plus className="h-3.5 w-3.5" />}
                      onClick={() => {
                        if (presetFormOpen) {
                          setPresetFormOpen(false);
                          setEditingPresetIdx(null);
                        } else {
                          openCreatePreset(false);
                        }
                      }}
                    >
                      {presetFormOpen ? "收起" : "新增预设"}
                    </Button>
                  </div>
                ) : (
                  <Button
                    type="button"
                    size="sm"
                    variant="secondary"
                    icon={<Plus className="h-3.5 w-3.5" />}
                    onClick={() =>
                      setAppTemplates((prev) => [
                        ...prev,
                        { path: "config/server.ini", content: "# {{serverName}}\n" },
                      ])
                    }
                  >
                    添加模板
                  </Button>
                )}
              </div>

              {presetsSubView === "playstyle" ? (
                <div>
                  {presetFormOpen && (
                    <div className="space-y-3 border-b border-[#EEF0F7] bg-[#F7F8FC] px-6 py-5">
                      <div className="text-xs font-semibold text-[#14161F]">
                        {editingPresetIdx === null ? "新增玩法预设" : "编辑玩法预设"}
                      </div>
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                        <Field label="预设标识 (ID)" htmlFor="preset-id">
                          <Input
                            id="preset-id"
                            disabled={editingPresetIdx !== null}
                            placeholder="casual-2x"
                            value={presetId}
                            className="h-9 font-mono text-xs"
                            onChange={(e) => setPresetId(e.target.value)}
                          />
                        </Field>
                        <Field label="预设名称" htmlFor="preset-name">
                          <Input
                            id="preset-name"
                            placeholder="双倍经验休闲服"
                            value={presetName}
                            className="h-9 text-xs"
                            onChange={(e) => setPresetName(e.target.value)}
                          />
                        </Field>
                        <Field label="预设简介" htmlFor="preset-desc">
                          <Input
                            id="preset-desc"
                            placeholder="双倍采集与经验、死亡不掉落"
                            value={presetDesc}
                            className="h-9 text-xs"
                            onChange={(e) => setPresetDesc(e.target.value)}
                          />
                        </Field>
                      </div>
                      <Field label="覆盖参数 (每行一个 字段Key=值)" htmlFor="preset-values">
                        <Textarea
                          id="preset-values"
                          rows={5}
                          className="font-mono text-xs"
                          placeholder={"expRate=2.0\ndeathPenalty=None"}
                          value={presetValuesText}
                          onChange={(e) => setPresetValuesText(e.target.value)}
                        />
                      </Field>
                      <div className="flex justify-end gap-2">
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          onClick={() => {
                            setPresetFormOpen(false);
                            setEditingPresetIdx(null);
                          }}
                        >
                          取消
                        </Button>
                        <Button type="button" size="sm" variant="primary" onClick={savePresetItem}>
                          保存预设
                        </Button>
                      </div>
                    </div>
                  )}

                  {appPresets.length === 0 ? (
                    <div className="px-6 py-12 text-center text-sm text-[#8A90A6]">
                      暂无预设，点击右上角「新增预设」添加一键玩法配置。
                    </div>
                  ) : (
                    <div className="divide-y divide-[#EEF0F7]">
                      {appPresets.map((p, idx) => (
                        <div
                          key={p.id}
                          className="flex flex-wrap items-center justify-between gap-4 px-6 py-4 transition-colors hover:bg-[#FAFBFD]"
                        >
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2">
                              <span className="font-medium text-[#14161F]">{p.name}</span>
                              <Mono className="text-xs text-[#8A90A6]">{p.id}</Mono>
                            </div>
                            {p.description && (
                              <div className="mt-0.5 truncate text-xs text-[#5B6075]">{p.description}</div>
                            )}
                          </div>
                          <div className="flex shrink-0 items-center gap-3">
                            <Badge tone="neutral">{Object.keys(p.values || {}).length} 项参数</Badge>
                            <div className="flex items-center gap-1">
                              <IconButton label={`编辑预设 ${p.name}`} onClick={() => openEditPreset(idx)}>
                                <Pencil className="h-3.5 w-3.5" aria-hidden />
                              </IconButton>
                              <IconButton
                                label={`删除预设 ${p.name}`}
                                onClick={() => setAppPresets((prev) => prev.filter((_, i) => i !== idx))}
                                className="hover:!bg-[#FEF3F2] hover:!text-[#D92D20]"
                              >
                                <Trash2 className="h-3.5 w-3.5" aria-hidden />
                              </IconButton>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ) : appTemplates.length === 0 ? (
                <div className="px-6 py-12 text-center text-sm text-[#8A90A6]">
                  未设置文件模板，仅注入环境变量。
                </div>
              ) : (
                <div className="divide-y divide-[#EEF0F7]">
                  {appTemplates.map((tpl, idx) => (
                    <div key={idx} className="space-y-3 px-6 py-5">
                      <div className="flex items-center justify-between gap-3">
                        <Input
                          value={tpl.path}
                          placeholder="工作目录相对路径，例如 Pal/Saved/Config/LinuxServer/PalWorldSettings.ini"
                          className="font-mono text-xs"
                          onChange={(e) =>
                            setAppTemplates((prev) =>
                              prev.map((x, i) => (i === idx ? { ...x, path: e.target.value } : x)),
                            )
                          }
                        />
                        <IconButton
                          label="删除模板"
                          onClick={() => setAppTemplates((prev) => prev.filter((_, i) => i !== idx))}
                          className="hover:!bg-[#FEF3F2] hover:!text-[#D92D20]"
                        >
                          <Trash2 className="h-4 w-4" />
                        </IconButton>
                      </div>
                      <Textarea
                        rows={8}
                        className="font-mono text-xs"
                        value={tpl.content}
                        placeholder="使用 {{fieldName}} 引用参数"
                        onChange={(e) =>
                          setAppTemplates((prev) =>
                            prev.map((x, i) => (i === idx ? { ...x, content: e.target.value } : x)),
                          )
                        }
                      />
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {modalTab === "commands" && (
            <AppConsoleActionsEditor
              actions={appConsoleActions}
              onChange={setAppConsoleActions}
              jsonMode={cmdJsonOpen}
              onCloseJsonMode={() => setCmdJsonOpen(false)}
              editingId={editingCmdId}
              onEditingIdChange={setEditingCmdId}
              onToast={(type, msg) => toast(type, msg)}
            />
          )}

          {modalTab === "metrics" && (
            <AppMetricsEditor
              metrics={appMetrics}
              onChange={setAppMetrics}
              schema={appSchema}
              consoleActions={appConsoleActions}
              jsonMode={metricsJsonOpen}
              onCloseJsonMode={() => setMetricsJsonOpen(false)}
              editingId={editingMetricId}
              onEditingIdChange={setEditingMetricId}
              onToast={(type, msg) => toast(type, msg)}
            />
          )}
        </Card>
      )}

      <ConfirmDialog
        open={!!deleteAppTarget}
        onClose={() => !deletingApp && setDeleteAppTarget(null)}
        onConfirm={() => void confirmDeleteApp()}
        loading={deletingApp}
        danger
        title={`删除游戏应用「${deleteAppTarget?.displayName ?? ""}」？`}
        body="删除后该游戏将不再出现在新建服务器向导中，已创建的服务器不受影响。"
        confirmLabel="确认删除"
      />
    </div>
  );
}

export default function AdminRuntimesPage() {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full rounded-[20px]" />}>
      <AdminRuntimesInner />
    </Suspense>
  );
}
