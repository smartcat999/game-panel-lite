"use client";

import { ReactNode, Suspense, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AlertCircle, Check, ChevronDown, Info, Plus, Search, Trash2 } from "lucide-react";
import { useConsole } from "../ConsoleProvider";
import { formatCpu, formatGB } from "../lib/format";
import { localizeApiError } from "../lib/session";
import {
  DEFAULT_GAME_COVER,
  RESOURCE_PLANS,
  appToPreset,
  compileAppConfig,
  defaultAppConfigValues,
  formatAppCategory,
  gameCover,
  resolveRichApp,
  resolveServerArtifact,
  sortAppCategories,
  type GamePreset,
} from "../lib/presets";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  Field,
  IconButton,
  Input,
  LinkButton,
  Mono,
  Segmented,
  Select,
  Skeleton,
  focusRing,
} from "../../components/console/ui";
import { GameConfigEditor } from "../../components/console/servers/GameConfigEditor";
import { Stepper } from "../../components/console/wizard/Stepper";
import {
  advancedFromPreset,
  belowRecommended,
  effectivePlanStorage,
  findPlan,
  nextEnvId,
  recommendedPlanId,
  suggestName,
  validateName,
  validatePort,
  type AdvancedSettings,
} from "../../components/console/wizard/helpers";

const STEPS = ["选游戏", "选配置", "命名与设置", "确认"];

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(" ");

export default function NewServerPage() {
  return (
    <Suspense fallback={<WizardSkeleton />}>
      <NewServerWizard />
    </Suspense>
  );
}

function WizardSkeleton() {
  return (
    <div className="mx-auto max-w-[880px] space-y-6">
      <Skeleton className="h-9 w-full" />
      <Skeleton className="mx-auto h-8 w-48" />
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="aspect-[4/5] rounded-[20px]" />
        ))}
      </div>
    </div>
  );
}

function NewServerWizard() {
  const router = useRouter();
  const params = useSearchParams();
  const {
    ready,
    tenant,
    tenantObj,
    instances,
    runtimeClasses,
    apps,
    loading,
    canManageTenant,
    apiFetch,
    toast,
    reloadInstances,
  } = useConsole();

  const [presetId, setPresetId] = useState<string | null>(null);

  const availablePresets = useMemo<GamePreset[]>(
    () => (apps || []).filter((a) => a.published !== false).map(appToPreset),
    [apps],
  );

  const resolvePreset = (id?: string | null) =>
    id ? availablePresets.find((p) => p.id === id) : undefined;

  const appsLoading = !ready || (loading && apps.length === 0);
  const [step, setStep] = useState(0);
  const [maxReached, setMaxReached] = useState(0);
  const [planChoice, setPlanChoice] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [adv, setAdv] = useState<AdvancedSettings | null>(null);
  const [categoryFilter, setCategoryFilter] = useState("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [hydratedGameParam, setHydratedGameParam] = useState(false);

  useEffect(() => {
    if (hydratedGameParam) return;
    const gameParam = params.get("game");
    if (!gameParam) {
      setHydratedGameParam(true);
      return;
    }
    if (appsLoading) return;
    setHydratedGameParam(true);
    const matched = availablePresets.find((p) => p.id === gameParam);
    if (!matched) return;
    const planParam = findPlan(params.get("plan"));
    const initStep = planParam ? 2 : 1;
    setPresetId(matched.id);
    setPlanChoice(planParam?.id ?? null);
    setName(suggestName(matched));
    setAdv(advancedFromPreset(matched));
    setStep(initStep);
    setMaxReached(initStep);
  }, [appsLoading, availablePresets, hydratedGameParam, params]);

  const categories = useMemo(
    () => sortAppCategories(availablePresets.map((p) => p.category || "survival")),
    [availablePresets],
  );

  useEffect(() => {
    if (categoryFilter !== "all" && categories.length > 0 && !categories.includes(categoryFilter)) {
      setCategoryFilter("all");
    }
  }, [categories, categoryFilter]);
  const filteredPresets = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    return availablePresets.filter((p) => {
      const cat = p.category || "survival";
      if (categoryFilter !== "all" && cat !== categoryFilter) return false;
      if (!q) return true;
      return (
        p.name.toLowerCase().includes(q) ||
        p.summary.toLowerCase().includes(q) ||
        formatAppCategory(cat).toLowerCase().includes(q) ||
        (p.tagline?.toLowerCase().includes(q) ?? false)
      );
    });
  }, [availablePresets, categoryFilter, searchQuery]);

  const selectedApp = useMemo(
    () => resolveRichApp(presetId, apps),
    [apps, presetId],
  );
  const [appVersion, setAppVersion] = useState<string>("");
  const [appConfigValues, setAppConfigValues] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!selectedApp) {
      setAppVersion("");
      setAppConfigValues({});
      return;
    }
    const ver = selectedApp.recommendedVersion || selectedApp.versions?.[0] || "";
    setAppVersion(ver);
    setAppConfigValues(defaultAppConfigValues(selectedApp));
  }, [selectedApp]);

  const preset = resolvePreset(presetId);

  const noWorkspace = !loading && !tenant;
  const noCreatePerm = !loading && !canManageTenant;
  const blocked = noWorkspace || noCreatePerm;

  const chosenPlan = findPlan(planChoice);
  const effectivePlanId = chosenPlan?.id ?? recommendedPlanId(preset) ?? RESOURCE_PLANS[0]?.id ?? "";
  const plan = findPlan(effectivePlanId);
  const recommendedId = recommendedPlanId(preset);

  // ----------------------------------------------------------- runtime
  const defaultRuntime = runtimeClasses.find((r) => r.isDefault) ?? runtimeClasses[0];
  const runtimeName = selectedApp?.runtimeClassName || defaultRuntime?.metadata.name || "docker";

  const isCustomContainer = preset?.category === "custom" || preset?.id === "custom-container";

  // -------------------------------------------------------- validation
  const takenNames = useMemo(() => instances.map((i) => i.metadata.name), [instances]);
  const nameError = validateName(name, takenNames);
  const targetPortError = adv && isCustomContainer ? validatePort(adv.targetPort, false) : null;
  const publishedPortError =
    adv && adv.publishedPortMode === "fixed" ? validatePort(adv.publishedPort, false) : null;
  const imageError = adv && isCustomContainer && !adv.image.trim() ? "请填写镜像地址" : null;
  const customContainerInvalid = !!(targetPortError || imageError);
  const advancedInvalid = !!publishedPortError;
  const missingRequiredFields = useMemo(
    () =>
      (selectedApp?.configSchema || []).filter(
        (f) => f.required && !(appConfigValues[f.name] || "").trim(),
      ),
    [selectedApp, appConfigValues],
  );

  const stepValid = [
    !!preset,
    !!preset && !!plan,
    !!preset && !!plan && !!adv && !nameError && !customContainerInvalid && !advancedInvalid && missingRequiredFields.length === 0,
    !!preset && !!plan && !!adv && !nameError && !customContainerInvalid && !advancedInvalid && missingRequiredFields.length === 0 && !!tenant,
  ];
  const canProceed = stepValid[step] && !blocked && !loading;

  // ----------------------------------------------------------- actions
  function goTo(i: number) {
    setSubmitError(null);
    setStep(i);
    setMaxReached((m) => Math.max(m, i));
    if (typeof window !== "undefined") window.scrollTo({ top: 0, behavior: "smooth" });
  }

  function selectPreset(id: string) {
    if (id === presetId) return;
    const p = resolvePreset(id);
    if (!p) return;
    setPresetId(id);
    setPlanChoice(null);
    setName(suggestName(p));
    setAdv(advancedFromPreset(p));
    setMaxReached(0);
  }

  function patchAdv(patch: Partial<AdvancedSettings>) {
    setAdv((cur) => (cur ? { ...cur, ...patch } : cur));
  }

  async function handleCreate() {
    if (!preset || !plan || !adv || !tenant || !stepValid[3] || blocked) return;
    const serverName = name.trim();
    setSubmitting(true);
    setSubmitError(null);
    try {
      const baseEnv = Object.fromEntries(
        adv.env.filter((e) => e.name.trim()).map((e) => [e.name.trim(), e.value]),
      );
      const compiled = selectedApp
        ? compileAppConfig(selectedApp, appVersion, appConfigValues, baseEnv)
        : null;
      const finalImage = isCustomContainer
        ? adv.image.trim()
        : (compiled?.image || selectedApp?.image || preset.image).trim();
      const finalCommand = isCustomContainer
        ? adv.command.trim()
          ? ["sh", "-c", adv.command.trim()]
          : undefined
        : selectedApp?.command?.length
          ? selectedApp.command
          : preset.command.trim()
            ? ["sh", "-c", preset.command.trim()]
            : undefined;
      const primaryPortName = isCustomContainer ? adv.portName || "game" : preset.portName || "game";
      const primaryProtocol = isCustomContainer ? adv.protocol : preset.protocol;
      const primaryTargetPort = isCustomContainer ? Number(adv.targetPort) : preset.targetPort;
      const publishedPort = adv.publishedPortMode === "fixed" ? Number(adv.publishedPort) : 0;
      const effectiveStorage = effectivePlanStorage(plan, preset);
      const finalEnv = compiled ? compiled.env : baseEnv;
      const finalFiles = compiled && compiled.files.length > 0 ? compiled.files : undefined;
      const serverArtifact = selectedApp ? resolveServerArtifact(selectedApp, appVersion) : undefined;
      const extraPorts =
        selectedApp?.ports && selectedApp.ports.length > 1
          ? selectedApp.ports.slice(1).map((p) => ({
              name: p.name,
              protocol: p.protocol,
              targetPort: p.targetPort,
              publishedPort: p.publishedPort ?? 0,
            }))
          : [];

      const res = await apiFetch(`/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/instances`, {
        method: "POST",
        body: JSON.stringify({
          apiVersion: "core.gamepanel.io/v1",
          kind: "Instance",
          metadata: {
            name: serverName,
            tenant,
            labels: {
              "gamepanel.io/preset": preset.id,
              "gamepanel.io/app": preset.id,
            },
            ...(selectedApp
              ? {
                  annotations: {
                    "core.gamepanel.io/app-config": JSON.stringify(appConfigValues),
                    ...(appVersion ? { "core.gamepanel.io/app-version": appVersion } : {}),
                  },
                }
              : {}),
          },
          spec: {
            runtimeClassName: runtimeName,
            state: "Running",
            image: finalImage,
            ...(finalCommand ? { command: finalCommand } : {}),
            ...(selectedApp?.workingDir ? { workingDir: selectedApp.workingDir } : {}),
            ...(serverArtifact ? { serverArtifact } : {}),
            env: finalEnv,
            ...(finalFiles ? { files: finalFiles } : {}),
            ports: [
              {
                name: primaryPortName,
                protocol: primaryProtocol,
                targetPort: primaryTargetPort,
                publishedPort,
              },
              ...extraPorts,
            ],
            resources: {
              requests: { cpu: plan.cpu, memory: plan.memory, storage: effectiveStorage },
              limits: { cpu: plan.cpu, memory: plan.memory, storage: effectiveStorage },
            },
            ...(selectedApp?.storagePolicy
              ? {
                  backupPolicy: {
                    enabled: false,
                    subtree: selectedApp.storagePolicy.backupSubtree,
                    replaceOnRestore: selectedApp.storagePolicy.replaceOnRestore,
                    backupRequiresStop: selectedApp.storagePolicy.backupRequiresStop,
                    preBackupCommand: selectedApp.storagePolicy.preBackupCommand,
                    postBackupCommand: selectedApp.storagePolicy.postBackupCommand,
                    ...(selectedApp.consoleDelivery
                      ? { consoleDelivery: selectedApp.consoleDelivery }
                      : {}),
                  },
                }
              : {}),
          },
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(localizeApiError(body?.message || "", res.status, "创建服务器失败"));
      }
      toast("success", `服务器 ${serverName} 已创建，正在启动`);
      void reloadInstances().catch(() => undefined);
      router.push(`/console/servers/${encodeURIComponent(serverName)}`);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "创建服务器失败，请稍后重试");
      setSubmitting(false);
    }
  }

  const workspaceLabel = tenantObj?.spec.displayName || tenant || "—";

  if (appsLoading && params.get("game")) {
    return <WizardSkeleton />;
  }

  return (
    <div className="mx-auto max-w-[880px]">
      <div className="overflow-x-auto pb-1">
        <Stepper steps={STEPS} current={step} maxReached={maxReached} onSelect={goTo} />
      </div>

      {noWorkspace && (
        <Notice tone="warning" className="mt-6">
          你还没有可用的工作区，暂时无法创建服务器。请联系管理员为你开通工作区。
        </Notice>
      )}
      {!noWorkspace && noCreatePerm && (
        <Notice tone="warning" className="mt-6">
          你在当前工作区「{workspaceLabel}」没有创建服务器的权限。可切换到你自己的工作区，或联系工作区管理员将你设为管理员。
        </Notice>
      )}

      <div className="mt-8">
        {step === 0 && (
          <section aria-labelledby="step-title">
            <StepTitle title="选择游戏" subtitle="选择游戏服务端模板，系统将自动匹配推荐镜像、端口与默认参数" />
            {appsLoading ? (
              <>
                <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
                  <Skeleton className="h-10 w-72 rounded-full" />
                  <Skeleton className="h-10 w-full rounded-xl sm:w-64" />
                </div>
                <div className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-3">
                  {Array.from({ length: 6 }).map((_, i) => (
                    <Skeleton key={i} className="aspect-[4/5] rounded-[20px]" />
                  ))}
                </div>
              </>
            ) : availablePresets.length === 0 ? (
              <div className="mt-6">
                <EmptyState
                  title="暂无可用游戏模板"
                  body="当前平台尚未上架游戏服务端模板，请联系管理员在应用市场开启上架。"
                />
              </div>
            ) : (
              <>
                <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
                  <Segmented
                    label="游戏分类"
                    value={categoryFilter}
                    onChange={setCategoryFilter}
                    items={[
                      { value: "all", label: "全部游戏" },
                      ...categories.map((c) => ({ value: c, label: formatAppCategory(c) })),
                    ]}
                  />
                  <div className="relative w-full sm:w-64">
                    <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[#8A90A6]" aria-hidden />
                    <Input
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      placeholder="搜索游戏名称或简介..."
                      className="pl-9"
                      aria-label="搜索游戏"
                    />
                  </div>
                </div>
                {filteredPresets.length === 0 ? (
                  <div className="mt-6">
                    <EmptyState
                      title="没有找到匹配的游戏"
                      body="换个关键词试试，或切回「全部游戏」查看所有模板。"
                      action={
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => {
                            setCategoryFilter("all");
                            setSearchQuery("");
                          }}
                        >
                          重置筛选
                        </Button>
                      }
                    />
                  </div>
                ) : (
                  <div className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-3">
                    {filteredPresets.map((p) => {
                      const selected = p.id === presetId;
                      return (
                        <button
                          key={p.id}
                          type="button"
                          aria-pressed={selected}
                          onClick={() => selectPreset(p.id)}
                          className={cx(
                            "group relative overflow-hidden rounded-[20px] border bg-white text-left transition-[border-color,box-shadow]",
                            selected
                              ? "border-[#2B3AE8] ring-2 ring-[#2B3AE8] shadow-[0_16px_32px_-20px_rgba(43,58,232,0.7)]"
                              : "border-[#E3E6F2] hover:border-[#B8C0E6]",
                            focusRing,
                          )}
                        >
                          {/* eslint-disable-next-line @next/next/no-img-element */}
                          <img
                            src={gameCover(p.id, p.coverImage)}
                            onError={(e) => {
                              if (!e.currentTarget.src.endsWith(DEFAULT_GAME_COVER)) {
                                e.currentTarget.src = DEFAULT_GAME_COVER;
                              }
                            }}
                            alt=""
                            className="aspect-[4/5] w-full rounded-t-[19px] object-cover transition-transform duration-300 group-hover:scale-[1.02]"
                          />
                          <div className="px-4 py-3">
                            <div className="truncate text-[15px] font-semibold text-[#14161F]">{p.name}</div>
                            <div className="mt-0.5 truncate text-[13px] text-[#5B6075]">{p.tagline || p.summary}</div>
                          </div>
                          {selected && (
                            <span className="absolute right-3 top-3 flex h-7 w-7 items-center justify-center rounded-full bg-[#2B3AE8] text-white shadow">
                              <Check className="h-4 w-4" strokeWidth={2.5} />
                            </span>
                          )}
                        </button>
                      );
                    })}
                  </div>
                )}
              </>
            )}
          </section>
        )}

        {step === 1 && preset && (
          <section>
            <div className="flex justify-center">
              <GameChip preset={preset} onChange={() => goTo(0)} />
            </div>
            <StepTitle title="选择配置" subtitle="已按该游戏的推荐配置预选，创建后可随时调整" className="mt-4" />
            <div className="mt-8 grid gap-4 sm:grid-cols-2">
              {RESOURCE_PLANS.map((p) => {
                const below = belowRecommended(p, preset);
                const selected = p.id === effectivePlanId;
                return (
                  <button
                    key={p.id}
                    type="button"
                    aria-pressed={selected}
                    onClick={() => setPlanChoice(p.id)}
                    className={cx(
                      "relative rounded-[20px] border bg-white p-5 text-left transition-[border-color,box-shadow]",
                      selected
                        ? "border-[#2B3AE8] ring-2 ring-[#2B3AE8] shadow-[0_16px_32px_-20px_rgba(43,58,232,0.7)]"
                        : "border-[#E3E6F2] hover:border-[#B8C0E6]",
                      focusRing,
                    )}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <span className="text-[16px] font-semibold text-[#14161F]">
                        {p.label}
                      </span>
                      {p.id === recommendedId && <Badge tone="accent">推荐</Badge>}
                    </div>
                    <div className="mt-1.5 text-[17px] font-medium tabular-nums text-[#14161F]">
                      {formatCpu(p.cpu)} {formatGB(p.memory)}
                    </div>
                    <div className="mt-1 text-[13px] text-[#5B6075]">
                      {formatGB(effectivePlanStorage(p, preset))} 存储
                    </div>
                    <div className="mt-3 flex h-6 items-center gap-2">
                      {selected && (
                        <span className="flex h-6 w-6 items-center justify-center rounded-full bg-[#2B3AE8] text-white">
                          <Check className="h-3.5 w-3.5" strokeWidth={2.5} />
                        </span>
                      )}
                      {below && <span className="text-[13px] text-[#B54708]">低于推荐配置，可能卡顿</span>}
                    </div>
                  </button>
                );
              })}
            </div>
          </section>
        )}

        {step === 2 && preset && adv && (
          <section>
            <div className="flex justify-center">
              <GameChip preset={preset} onChange={() => goTo(0)} />
            </div>
            <StepTitle title="命名与设置" subtitle="设置服务器名称，并按需调整游戏参数或玩法预设" className="mt-4" />
            <Card className="mt-8 p-6">
              <Field
                label="服务器名称"
                htmlFor="server-name"
                error={nameError}
                hint="小写字母、数字和 -，3–40 个字符，以字母开头。创建后不能修改。"
              >
                <Input
                  id="server-name"
                  value={name}
                  onChange={(e) => setName(e.target.value.toLowerCase())}
                  autoComplete="off"
                  autoCorrect="off"
                  autoCapitalize="none"
                  spellCheck={false}
                  maxLength={40}
                  aria-invalid={!!nameError}
                />
              </Field>

              {selectedApp && (selectedApp.configSchema?.length || 0) > 0 && (
                <div className="mt-6 border-t border-[#EEF0F7] pt-5">
                  <GameConfigEditor
                    app={selectedApp}
                    values={appConfigValues}
                    onChange={setAppConfigValues}
                    version={appVersion}
                    syncUrlParams={false}
                    onVersionChange={setAppVersion}
                  />
                  {missingRequiredFields.length > 0 && (
                    <Notice tone="warning" className="mt-4">
                      请先填写必填游戏参数：{missingRequiredFields.map((f) => f.label || f.name).join("、")}
                    </Notice>
                  )}
                </div>
              )}

              {isCustomContainer && (
                <div className="mt-6 grid gap-5 border-t border-[#EEF0F7] pt-5 sm:grid-cols-2">
                  <div className="sm:col-span-2">
                    <Field label="镜像" htmlFor="custom-image" error={imageError}>
                      <Input
                        id="custom-image"
                        className="font-mono"
                        value={adv.image}
                        autoCorrect="off"
                        autoCapitalize="none"
                        spellCheck={false}
                        aria-invalid={!!imageError}
                        onChange={(e) => patchAdv({ image: e.target.value })}
                      />
                    </Field>
                  </div>
                  <div className="sm:col-span-2">
                    <Field label="启动命令" htmlFor="custom-command">
                      <Input
                        id="custom-command"
                        className="font-mono"
                        placeholder="留空则使用镜像默认启动命令"
                        value={adv.command}
                        autoCorrect="off"
                        autoCapitalize="none"
                        spellCheck={false}
                        onChange={(e) => patchAdv({ command: e.target.value })}
                      />
                    </Field>
                  </div>
                  <Field label="协议" htmlFor="custom-proto">
                    <Select
                      id="custom-proto"
                      value={adv.protocol}
                      onChange={(e) => patchAdv({ protocol: e.target.value as "TCP" | "UDP" })}
                    >
                      <option value="TCP">TCP</option>
                      <option value="UDP">UDP</option>
                    </Select>
                  </Field>
                  <Field label="服务端口" htmlFor="custom-target" error={targetPortError}>
                    <Input
                      id="custom-target"
                      type="text"
                      inputMode="numeric"
                      autoCorrect="off"
                      autoCapitalize="none"
                      spellCheck={false}
                      aria-invalid={!!targetPortError}
                      className="font-mono"
                      value={adv.targetPort}
                      onChange={(e) => patchAdv({ targetPort: e.target.value })}
                    />
                  </Field>
                </div>
              )}

              <div className="mt-6 border-t border-[#EEF0F7] pt-4">
                <button
                  type="button"
                  aria-expanded={showAdvanced}
                  aria-controls="advanced-settings"
                  onClick={() => setShowAdvanced((v) => !v)}
                  className={cx(
                    "flex w-full items-center justify-between rounded-xl py-2 text-left text-sm font-medium text-[#14161F]",
                    focusRing,
                  )}
                >
                  <span>
                    高级设置
                    {advancedInvalid && !showAdvanced && (
                      <span className="ml-2 text-[13px] font-normal text-[#B42318]">有需要修改的项</span>
                    )}
                  </span>
                  <ChevronDown className={cx("h-4 w-4 text-[#5B6075] transition-transform", showAdvanced && "rotate-180")} />
                </button>

                {showAdvanced && (
                  <div id="advanced-settings" className="mt-4 space-y-5">
                    <div>
                      <div className="mb-1.5 block text-[13px] font-medium text-[#3D4258]">连接端口</div>
                      <Segmented
                        size="sm"
                        label="连接端口分配方式"
                        value={adv.publishedPortMode}
                        onChange={(mode) => patchAdv({ publishedPortMode: mode })}
                        items={[
                          { value: "auto", label: "自动分配" },
                          { value: "fixed", label: "指定端口" },
                        ]}
                      />
                      {adv.publishedPortMode === "auto" ? (
                        <p className="mt-2 text-[13px] text-[#8A90A6]">
                          启动时自动从节点可用端口池分配空闲端口。
                        </p>
                      ) : (
                        <div className="mt-3">
                          <Field
                            label="指定连接端口"
                            htmlFor="adv-published"
                            error={publishedPortError}
                            hint="端口范围 1–65535"
                          >
                            <Input
                              id="adv-published"
                              type="text"
                              inputMode="numeric"
                              autoCorrect="off"
                              autoCapitalize="none"
                              spellCheck={false}
                              aria-invalid={!!publishedPortError}
                              className="font-mono sm:max-w-[200px]"
                              value={adv.publishedPort}
                              onChange={(e) => patchAdv({ publishedPort: e.target.value })}
                            />
                          </Field>
                        </div>
                      )}
                    </div>
                    <EnvEditor rows={adv.env} onChange={(env) => patchAdv({ env })} />
                  </div>
                )}
              </div>
            </Card>
          </section>
        )}

        {step === 3 && preset && plan && adv && (
          <section>
            <StepTitle title="确认创建" subtitle="确认无误后点击创建，服务器会自动启动" />
            <Card className="mt-8">
              <dl className="divide-y divide-[#EEF0F7]">
                <SummaryRow label="游戏">
                  <span className="flex items-center gap-3">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={gameCover(preset.id, preset.coverImage)}
                      onError={(e) => {
                        if (!e.currentTarget.src.endsWith(DEFAULT_GAME_COVER)) {
                          e.currentTarget.src = DEFAULT_GAME_COVER;
                        }
                      }}
                      alt=""
                      className="h-10 w-8 rounded-lg object-cover"
                    />
                    <span>
                      <span className="block font-medium text-[#14161F]">{preset.name}</span>
                      {isCustomContainer && (
                        <Mono className="text-[12px] text-[#5B6075]">{adv.image}</Mono>
                      )}
                    </span>
                  </span>
                </SummaryRow>
                {appVersion && (
                  <SummaryRow label="游戏版本">
                    <Mono>{appVersion}</Mono>
                  </SummaryRow>
                )}
                <SummaryRow label="配置">
                  <span className="font-medium text-[#14161F]">{plan.label}</span>
                  <span className="ml-2 text-[#5B6075]">
                    {formatCpu(plan.cpu)} {formatGB(plan.memory)}，{formatGB(effectivePlanStorage(plan, preset))} 存储
                  </span>
                </SummaryRow>
                <SummaryRow label="名称">
                  <span className="font-medium text-[#14161F]">{name.trim()}</span>
                </SummaryRow>
                <SummaryRow label="工作区">{workspaceLabel}</SummaryRow>
                {isCustomContainer ? (
                  <SummaryRow label="端口">
                    服务端口 <Mono>{adv.targetPort}/{adv.protocol}</Mono>，
                    {adv.publishedPortMode === "auto" ? (
                      "连接端口自动分配"
                    ) : (
                      <>
                        连接端口固定为 <Mono>{adv.publishedPort.trim()}</Mono>
                      </>
                    )}
                  </SummaryRow>
                ) : (
                  <SummaryRow label="连接端口">
                    {adv.publishedPortMode === "auto" ? (
                      "自动分配"
                    ) : (
                      <>
                        指定端口 <Mono>{adv.publishedPort.trim()}</Mono>
                      </>
                    )}
                  </SummaryRow>
                )}
              </dl>
            </Card>
            {submitError && (
              <div
                role="alert"
                className="mt-4 flex items-start gap-2.5 rounded-2xl border border-[#F4C7C3] bg-[#FEF3F2] px-4 py-3 text-sm text-[#B42318]"
              >
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                <div>
                  <div className="font-medium">创建失败</div>
                  <div className="mt-0.5 break-words">{submitError}</div>
                </div>
              </div>
            )}
          </section>
        )}
      </div>

      <div className="sticky bottom-4 z-10 mt-8 flex items-center justify-between gap-3 rounded-full border border-[#E3E6F2] bg-white/90 p-2 shadow-[0_16px_40px_-24px_rgba(20,22,31,0.45)] backdrop-blur">
        {step === 0 ? (
          <LinkButton href="/console" variant="ghost">
            取消
          </LinkButton>
        ) : (
          <Button variant="ghost" onClick={() => goTo(step - 1)} disabled={submitting}>
            上一步
          </Button>
        )}
        {step < 3 ? (
          <Button variant="primary" disabled={!canProceed} onClick={() => goTo(step + 1)}>
            下一步
          </Button>
        ) : (
          <Button variant="primary" disabled={!canProceed} loading={submitting} onClick={() => void handleCreate()}>
            创建服务器
          </Button>
        )}
      </div>
    </div>
  );
}

// ------------------------------------------------------------- pieces
function StepTitle({ title, subtitle, className }: { title: string; subtitle: string; className?: string }) {
  return (
    <div className={cx("text-center", className)}>
      <h1 id="step-title" className="text-[28px] font-bold tracking-tight text-[#14161F]">
        {title}
      </h1>
      <p className="mt-1.5 text-[15px] text-[#5B6075]">{subtitle}</p>
    </div>
  );
}

function GameChip({ preset, onChange }: { preset: GamePreset; onChange: () => void }) {
  return (
    <div className="inline-flex items-center gap-2.5 rounded-full border border-[#E3E6F2] bg-white py-1 pl-1 pr-2 shadow-sm">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={gameCover(preset.id, preset.coverImage)}
        onError={(e) => {
          if (!e.currentTarget.src.endsWith(DEFAULT_GAME_COVER)) {
            e.currentTarget.src = DEFAULT_GAME_COVER;
          }
        }}
        alt=""
        className="h-8 w-8 rounded-full object-cover"
      />
      <span className="text-sm font-semibold text-[#14161F]">{preset.name}</span>
      <button
        type="button"
        onClick={onChange}
        className={cx("rounded-full px-2 py-1 text-[13px] font-medium text-[#2B3AE8] hover:bg-[#EEF1FF]", focusRing)}
      >
        更换
      </button>
    </div>
  );
}

function Notice({
  tone,
  className,
  children,
}: {
  tone: "info" | "warning";
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      role={tone === "warning" ? "alert" : "note"}
      className={cx(
        "flex items-start gap-2.5 rounded-2xl border px-4 py-3 text-sm",
        tone === "info" ? "border-[#D9DEFB] bg-[#F3F5FF] text-[#3D4258]" : "border-[#FEDF89] bg-[#FFFAEB] text-[#B54708]",
        className,
      )}
    >
      {tone === "info" ? (
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-[#2B3AE8]" />
      ) : (
        <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
      )}
      <div className="min-w-0">{children}</div>
    </div>
  );
}

function SummaryRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 px-6 py-4 sm:flex-row sm:items-center sm:gap-6">
      <dt className="w-24 shrink-0 text-[13px] text-[#5B6075]">{label}</dt>
      <dd className="min-w-0 break-words text-sm text-[#14161F]">{children}</dd>
    </div>
  );
}

function EnvEditor({
  rows,
  onChange,
}: {
  rows: AdvancedSettings["env"];
  onChange: (rows: AdvancedSettings["env"]) => void;
}) {
  const update = (id: number, patch: Partial<{ name: string; value: string }>) =>
    onChange(rows.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  return (
    <div>
      <div className="flex items-center justify-between gap-3">
        <span className="text-[13px] font-medium text-[#3D4258]">环境变量</span>
        <Button
          size="sm"
          variant="ghost"
          icon={<Plus className="h-3.5 w-3.5" />}
          onClick={() => onChange([...rows, { id: nextEnvId(), name: "", value: "" }])}
        >
          添加变量
        </Button>
      </div>
      {rows.length === 0 ? (
        <p className="mt-2 text-[13px] text-[#8A90A6]">没有环境变量。需要时点击“添加变量”。</p>
      ) : (
        <div className="mt-2 space-y-2">
          {rows.map((r, idx) => (
            <div key={r.id} className="flex items-center gap-2">
              <div className="w-[40%] min-w-0">
                <Input
                  aria-label={`第 ${idx + 1} 个变量名`}
                  placeholder="名称"
                  className="font-mono"
                  value={r.name}
                  autoCorrect="off"
                  autoCapitalize="none"
                  spellCheck={false}
                  onChange={(e) => update(r.id, { name: e.target.value })}
                />
              </div>
              <div className="min-w-0 flex-1">
                <Input
                  aria-label={`第 ${idx + 1} 个变量值`}
                  placeholder="值"
                  className="font-mono"
                  value={r.value}
                  autoCorrect="off"
                  autoCapitalize="none"
                  spellCheck={false}
                  onChange={(e) => update(r.id, { value: e.target.value })}
                />
              </div>
              <IconButton label={`删除变量 ${r.name || idx + 1}`} onClick={() => onChange(rows.filter((x) => x.id !== r.id))}>
                <Trash2 className="h-4 w-4" />
              </IconButton>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
