"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { FilePlus2, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import type {
  AddonConfigField,
  FileEntry,
  InstanceAddon,
  RegistryArtifact,
  RegistryPackage,
} from "../../../console/lib/types";
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  IconButton,
  Input,
  Modal,
  Mono,
  Segmented,
  Select,
  Skeleton,
  Switch,
  Textarea,
} from "../ui";
import { useServerFiles } from "./api";

const CONFIG_EXT_RE = /\.(ya?ml|json|toml|properties|conf|cfg|ini)$/i;

function sanitizeStem(raw: string): string {
  return raw.replace(/[^A-Za-z0-9._-]+/g, "").replace(/^[._-]+|[._-]+$/g, "");
}

function normalizeToken(raw: string): string {
  return raw.toLowerCase().replace(/[^a-z0-9]+/g, "");
}

function sanitizeJsonFilename(raw: string): string {
  const base = raw.trim().split(/[/\\]/).pop() || "";
  const cleaned = base.replace(/[^A-Za-z0-9._-]+/g, "_").replace(/^\.+/, "");
  if (!cleaned) return "Mod_ServerConfig.json";
  return cleaned.toLowerCase().endsWith(".json") ? cleaned : `${cleaned}.json`;
}

function defaultTModFilename(modIds: string[], title: string): string {
  const stem = sanitizeStem(modIds[0] || "") || sanitizeStem(title) || "Mod";
  return `${stem}_ServerConfig.json`;
}

function isEditablePluginConfig(name: string): boolean {
  return CONFIG_EXT_RE.test(name);
}

function parsePlainJsonObject(raw: string): Record<string, unknown> | null {
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    /* handled cleanly by JSON syntax validation */
  }
  return null;
}

async function listDirIfPresent(
  listFn: (dir: string) => Promise<FileEntry[]>,
  relDir: string,
): Promise<{ exists: boolean; entries: FileEntry[] }> {
  const parts = relDir.split("/").filter(Boolean);
  let cur = ".";
  for (const part of parts) {
    const entries = await listFn(cur);
    const found = entries.find((e) => e.isDir && e.name === part);
    if (!found) {
      return { exists: false, entries: [] };
    }
    cur = cur === "." ? part : `${cur}/${part}`;
  }
  const finalEntries = await listFn(cur || ".");
  return { exists: true, entries: finalEntries };
}

async function ensureDirTree(
  listFn: (dir: string) => Promise<FileEntry[]>,
  mkdirFn: (dir: string) => Promise<void>,
  relDir: string,
): Promise<void> {
  const parts = relDir.split("/").filter(Boolean);
  let cur = ".";
  for (const part of parts) {
    const entries = await listFn(cur);
    const exists = entries.some((e) => e.isDir && e.name === part);
    const next = cur === "." ? part : `${cur}/${part}`;
    if (!exists) {
      await mkdirFn(next);
    }
    cur = next;
  }
}

export interface AddonConfigModalProps {
  open: boolean;
  onClose: () => void;
  instName: string;
  running: boolean;
  noun: string;
  addon: InstanceAddon | null;
  pkgMeta?: RegistryPackage;
  artifact?: RegistryArtifact;
  isAdmin: boolean;
  retrying: boolean;
  onResyncArtifact?: (art: RegistryArtifact) => Promise<void>;
  onSaveSchemaConfig: (pkgId: string, nextConfig: Record<string, string> | undefined) => Promise<boolean>;
  savingSchema: boolean;
}

export function AddonConfigModal({
  open,
  onClose,
  instName,
  running,
  noun,
  addon,
  pkgMeta,
  artifact,
  isAdmin,
  retrying,
  onResyncArtifact,
  onSaveSchemaConfig,
  savingSchema,
}: AddonConfigModalProps) {
  const { toast } = useConsole();
  const { list: listFiles, read: readFile, write: writeFile, mkdir: mkdirPath } = useServerFiles(instName);

  const schema: AddonConfigField[] = useMemo(() => {
    if (artifact?.status?.configSchema?.length) return artifact.status.configSchema;
    return pkgMeta?.configSchema ?? [];
  }, [artifact?.status?.configSchema, pkgMeta?.configSchema]);

  const modIds = useMemo(() => {
    if (artifact?.status?.modIds?.length) return artifact.status.modIds;
    return pkgMeta?.modIds ?? [];
  }, [artifact?.status?.modIds, pkgMeta?.modIds]);

  const displayTitle = pkgMeta?.title || addon?.title || addon?.package || `${noun}配置`;
  const installMode = addon?.installMode || "";
  const isTMod = installMode === "tmod_latest";
  const isBepInEx = installMode === "bepinex";
  const isOxide = installMode === "oxide";
  const isSML = installMode === "sml";
  const isCS2Plugin = installMode === "cs2_plugin";
  const isFlatDir = installMode === "flat_dir" || installMode === "";
  const isFileMode = schema.length === 0 && (isTMod || isBepInEx || isOxide || isSML || isCS2Plugin || isFlatDir);

  // ----------------------------- Schema Form State -----------------------------
  const [schemaDraft, setSchemaDraft] = useState<Record<string, string>>({});
  const [schemaFilter, setSchemaFilter] = useState("");

  useEffect(() => {
    if (!open || !addon || schema.length === 0) return;
    const next: Record<string, string> = {};
    for (const f of schema) {
      const configured = addon.config?.[f.name];
      next[f.name] = configured !== undefined ? configured : (f.default ?? "");
    }
    setSchemaDraft(next);
    setSchemaFilter("");
  }, [open, addon, schema]);

  const schemaModifiedCount = useMemo(() => {
    let count = 0;
    for (const f of schema) {
      if ((schemaDraft[f.name] ?? "") !== (f.default ?? "")) {
        count++;
      }
    }
    return count;
  }, [schema, schemaDraft]);

  const visibleSchemaFields = useMemo(() => {
    const q = schemaFilter.trim().toLowerCase();
    if (!q) return schema;
    return schema.filter(
      (f) =>
        f.name.toLowerCase().includes(q) ||
        f.label.toLowerCase().includes(q) ||
        (f.description || "").toLowerCase().includes(q),
    );
  }, [schema, schemaFilter]);

  const handleResetAllSchema = useCallback(() => {
    const next: Record<string, string> = {};
    for (const f of schema) {
      next[f.name] = f.default ?? "";
    }
    setSchemaDraft(next);
  }, [schema]);

  const handleSaveSchema = useCallback(async () => {
    if (!addon) return;
    const sparse: Record<string, string> = {};
    for (const f of schema) {
      const cur = schemaDraft[f.name] ?? "";
      const def = f.default ?? "";
      if (cur !== def) {
        sparse[f.name] = cur;
      }
    }
    const ok = await onSaveSchemaConfig(
      addon.package,
      Object.keys(sparse).length > 0 ? sparse : undefined,
    );
    if (ok) onClose();
  }, [addon, onClose, onSaveSchemaConfig, schema, schemaDraft]);

  // --------------------------- Workspace File State ----------------------------
  const [filesLoading, setFilesLoading] = useState(false);
  const [filesError, setFilesError] = useState<string | null>(null);
  const [matchedFiles, setMatchedFiles] = useState<string[]>([]);
  const [fuzzyNotice, setFuzzyNotice] = useState<string | null>(null);
  const [selectedFile, setSelectedFile] = useState<string>("");
  const [fileLoading, setFileLoading] = useState(false);
  const [fileContent, setFileContent] = useState("");
  const [fileSavedContent, setFileSavedContent] = useState("");
  const [fileSaving, setFileSaving] = useState(false);
  const [fileEditView, setFileEditView] = useState<"form" | "json">("form");
  const [fileParseError, setFileParseError] = useState<string | null>(null);
  const [creatingOpen, setCreatingOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [creatingBusy, setCreatingBusy] = useState(false);
  const [detectedModIds, setDetectedModIds] = useState<string[]>([]);
  const [confirmExtractOpen, setConfirmExtractOpen] = useState(false);

  const effectiveModIds = useMemo(
    () => (modIds.length > 0 ? modIds : detectedModIds),
    [modIds, detectedModIds],
  );

  useEffect(() => {
    if (!open) {
      setConfirmExtractOpen(false);
      setDetectedModIds([]);
    }
  }, [open]);

  const scanWorkspaceConfigs = useCallback(async () => {
    if (!open || !addon || !isFileMode) return;
    setFilesLoading(true);
    setFilesError(null);
    setFuzzyNotice(null);
    setCreatingOpen(false);
    try {
      if (isTMod) {
        let resolvedIds = modIds;
        if (resolvedIds.length === 0) {
          const modsDir = await listDirIfPresent(listFiles, "Mods");
          const pkgTok = normalizeToken(addon.package);
          const titleTok = normalizeToken(displayTitle);
          const matchedTMod = modsDir.entries
            .filter((e) => !e.isDir && e.name.toLowerCase().endsWith(".tmod"))
            .map((e) => e.name.slice(0, -5))
            .filter((stem) => {
              const tok = normalizeToken(stem);
              if (!tok) return false;
              if (pkgTok && (tok === pkgTok || tok.includes(pkgTok) || pkgTok.includes(tok))) return true;
              return titleTok.length >= 3 && (tok === titleTok || tok.includes(titleTok) || titleTok.includes(tok));
            });
          if (matchedTMod.length > 0) {
            resolvedIds = matchedTMod;
          }
        }
        setDetectedModIds(resolvedIds);

        const res = await listDirIfPresent(listFiles, "SaveData/ModConfigs");
        const allJson = res.entries
          .filter((e) => !e.isDir && e.name.toLowerCase().endsWith(".json"))
          .map((e) => e.name)
          .sort((a, b) => a.localeCompare(b));

        let hits: string[] = [];
        if (resolvedIds.length > 0) {
          const lowerIds = resolvedIds.map((id) => id.toLowerCase());
          hits = allJson.filter((name) => {
            const low = name.toLowerCase();
            return lowerIds.some((id) => low === `${id}.json` || low.startsWith(`${id}_`));
          });
        } else if (isSML) {
        const configDir = "FactoryGame/Configs";
        const res = await listDirIfPresent(listFiles, configDir);
        const allCfgs = res.entries
          .filter((e) => !e.isDir && isEditablePluginConfig(e.name))
          .map((e) => e.name)
          .sort((a, b) => a.localeCompare(b));
        const pkgTok = normalizeToken(addon.package);
        const titleTok = normalizeToken(displayTitle);
        let hits = allCfgs.filter((name) => {
          const tok = normalizeToken(name.replace(CONFIG_EXT_RE, ""));
          if (!tok) return false;
          if (pkgTok.length >= 2 && (tok === pkgTok || tok.includes(pkgTok) || pkgTok.includes(tok))) return true;
          return titleTok.length >= 3 && (tok === titleTok || tok.includes(titleTok));
        });
        if (hits.length === 0 && allCfgs.length > 0) {
          hits = allCfgs;
          setFuzzyNotice(`未精确匹配到单一配置文件，已列出 ${configDir} 目录中的全部配置文件。`);
        }
        const paths = hits.map((name) => `${configDir}/${name}`);
        setMatchedFiles(paths);
        setSelectedFile((prev) => (prev && paths.includes(prev) ? prev : (paths[0] || "")));
      } else if (isCS2Plugin) {
        const baseDir = (addon.targetDir || "game/csgo/addons").replace(/^\/+|\/+$/g, "") || "game/csgo/addons";
        const csgoRoot = baseDir.endsWith("/addons") ? baseDir.slice(0, -"/addons".length) : "game/csgo";
        const cssConfigDir = `${baseDir}/counterstrikesharp/configs`;
        const shortPkg = addon.package.includes("--") ? addon.package.split("--").slice(1).join("--") : addon.package;
        const tokens = [normalizeToken(shortPkg), normalizeToken(addon.package), normalizeToken(displayTitle)].filter(
          (tok) => tok.length >= 3,
        );
        const collected: string[] = [];
        if (addon.package.toLowerCase() === "counterstrikesharp") {
          const rootCfgs = await listDirIfPresent(listFiles, cssConfigDir);
          for (const e of rootCfgs.entries) {
            if (!e.isDir && !e.name.toLowerCase().includes(".example.") && isEditablePluginConfig(e.name)) {
              collected.push(`${cssConfigDir}/${e.name}`);
            }
          }
        }
        const pluginsCfgRes = await listDirIfPresent(listFiles, `${cssConfigDir}/plugins`);
        for (const d of pluginsCfgRes.entries.filter((e) => e.isDir)) {
          const tok = normalizeToken(d.name);
          if (tokens.some((cand) => tok === cand || tok.includes(cand) || cand.includes(tok))) {
            const sub = await listFiles(`${cssConfigDir}/plugins/${d.name}`);
            for (const f of sub) {
              if (!f.isDir && isEditablePluginConfig(f.name)) {
                collected.push(`${cssConfigDir}/plugins/${d.name}/${f.name}`);
              }
            }
          }
        }
        const csgoCfgRes = await listDirIfPresent(listFiles, `${csgoRoot}/cfg`);
        for (const d of csgoCfgRes.entries.filter((e) => e.isDir)) {
          const tok = normalizeToken(d.name);
          if (tokens.some((cand) => tok === cand || tok.includes(cand) || cand.includes(tok))) {
            const sub = await listFiles(`${csgoRoot}/cfg/${d.name}`);
            for (const f of sub) {
              if (!f.isDir && isEditablePluginConfig(f.name)) {
                collected.push(`${csgoRoot}/cfg/${d.name}/${f.name}`);
              }
            }
          }
        }
        collected.sort((a, b) => a.localeCompare(b));
        setMatchedFiles(collected);
        setSelectedFile((prev) => (prev && collected.includes(prev) ? prev : (collected[0] || "")));
      } else {
          const titleTok = normalizeToken(displayTitle);
          if (titleTok.length >= 3) {
            hits = allJson.filter((name) => normalizeToken(name).includes(titleTok));
          }
          if (hits.length === 0 && allJson.length > 0) {
            hits = allJson;
            setFuzzyNotice("未获取到模组内部标识，已列出 SaveData/ModConfigs 目录中的配置文件。");
          }
        }
        const paths = hits.map((name) => `SaveData/ModConfigs/${name}`);
        setMatchedFiles(paths);
        setSelectedFile((prev) => (prev && paths.includes(prev) ? prev : (paths[0] || "")));
      } else if (isBepInEx) {
        const baseDir = (addon.targetDir || "BepInEx").replace(/^\/+|\/+$/g, "") || "BepInEx";
        const configDir = `${baseDir}/config`;
        const res = await listDirIfPresent(listFiles, configDir);
        const allCfgs = res.entries
          .filter((e) => !e.isDir && isEditablePluginConfig(e.name))
          .map((e) => e.name)
          .sort((a, b) => a.localeCompare(b));
        const pkgTok = normalizeToken(addon.package);
        const shortPkg = addon.package.includes("-") ? addon.package.split("-").slice(1).join("-") : addon.package;
        const shortTok = normalizeToken(shortPkg);
        const titleTok = normalizeToken(displayTitle);
        let hits = allCfgs.filter((name) => {
          const tok = normalizeToken(name);
          if (!tok) return false;
          if (pkgTok.length >= 3 && tok.includes(pkgTok)) return true;
          if (shortTok.length >= 3 && tok.includes(shortTok)) return true;
          return titleTok.length >= 3 && tok.includes(titleTok);
        });
        if (hits.length === 0 && allCfgs.length > 0) {
          hits = allCfgs;
          setFuzzyNotice(`未精确匹配到单一配置文件，已列出 ${configDir} 目录中的全部配置文件。`);
        }
        const paths = hits.map((name) => `${configDir}/${name}`);
        setMatchedFiles(paths);
        setSelectedFile((prev) => (prev && paths.includes(prev) ? prev : (paths[0] || "")));
      } else if (isOxide) {
        const baseDir = (addon.targetDir || "oxide").replace(/^\/+|\/+$/g, "") || "oxide";
        const configDir = `${baseDir}/config`;
        const res = await listDirIfPresent(listFiles, configDir);
        const allCfgs = res.entries
          .filter((e) => !e.isDir && isEditablePluginConfig(e.name))
          .map((e) => e.name)
          .sort((a, b) => a.localeCompare(b));
        const tokens = [
          normalizeToken(addon.package),
          normalizeToken(displayTitle),
          normalizeToken((artifact?.status?.descriptor?.fileName || "").replace(/\.cs$/i, "")),
        ].filter((tok) => tok.length >= 3);
        let hits = allCfgs.filter((name) => {
          const tok = normalizeToken(name.replace(CONFIG_EXT_RE, ""));
          return tok !== "" && tokens.some((cand) => tok === cand || tok.includes(cand) || cand.includes(tok));
        });
        if (hits.length === 0 && allCfgs.length > 0) {
          hits = allCfgs;
          setFuzzyNotice(`未精确匹配到单一配置文件，已列出 ${configDir} 目录中的全部配置文件。`);
        }
        const paths = hits.map((name) => `${configDir}/${name}`);
        setMatchedFiles(paths);
        setSelectedFile((prev) => (prev && paths.includes(prev) ? prev : (paths[0] || "")));
      } else if (isSML) {
        const configDir = "FactoryGame/Configs";
        const res = await listDirIfPresent(listFiles, configDir);
        const allCfgs = res.entries
          .filter((e) => !e.isDir && isEditablePluginConfig(e.name))
          .map((e) => e.name)
          .sort((a, b) => a.localeCompare(b));
        const pkgTok = normalizeToken(addon.package);
        const titleTok = normalizeToken(displayTitle);
        let hits = allCfgs.filter((name) => {
          const tok = normalizeToken(name.replace(CONFIG_EXT_RE, ""));
          if (!tok) return false;
          if (pkgTok.length >= 2 && (tok === pkgTok || tok.includes(pkgTok) || pkgTok.includes(tok))) return true;
          return titleTok.length >= 3 && (tok === titleTok || tok.includes(titleTok));
        });
        if (hits.length === 0 && allCfgs.length > 0) {
          hits = allCfgs;
          setFuzzyNotice(`未精确匹配到单一配置文件，已列出 ${configDir} 目录中的全部配置文件。`);
        }
        const paths = hits.map((name) => `${configDir}/${name}`);
        setMatchedFiles(paths);
        setSelectedFile((prev) => (prev && paths.includes(prev) ? prev : (paths[0] || "")));
      } else if (isCS2Plugin) {
        const baseDir = (addon.targetDir || "game/csgo/addons").replace(/^\/+|\/+$/g, "") || "game/csgo/addons";
        const csgoRoot = baseDir.endsWith("/addons") ? baseDir.slice(0, -"/addons".length) : "game/csgo";
        const cssConfigDir = `${baseDir}/counterstrikesharp/configs`;
        const shortPkg = addon.package.includes("--") ? addon.package.split("--").slice(1).join("--") : addon.package;
        const tokens = [normalizeToken(shortPkg), normalizeToken(addon.package), normalizeToken(displayTitle)].filter(
          (tok) => tok.length >= 3,
        );
        const collected: string[] = [];
        if (addon.package.toLowerCase() === "counterstrikesharp") {
          const rootCfgs = await listDirIfPresent(listFiles, cssConfigDir);
          for (const e of rootCfgs.entries) {
            if (!e.isDir && !e.name.toLowerCase().includes(".example.") && isEditablePluginConfig(e.name)) {
              collected.push(`${cssConfigDir}/${e.name}`);
            }
          }
        }
        const pluginsCfgRes = await listDirIfPresent(listFiles, `${cssConfigDir}/plugins`);
        for (const d of pluginsCfgRes.entries.filter((e) => e.isDir)) {
          const tok = normalizeToken(d.name);
          if (tokens.some((cand) => tok === cand || tok.includes(cand) || cand.includes(tok))) {
            const sub = await listFiles(`${cssConfigDir}/plugins/${d.name}`);
            for (const f of sub) {
              if (!f.isDir && isEditablePluginConfig(f.name)) {
                collected.push(`${cssConfigDir}/plugins/${d.name}/${f.name}`);
              }
            }
          }
        }
        const csgoCfgRes = await listDirIfPresent(listFiles, `${csgoRoot}/cfg`);
        for (const d of csgoCfgRes.entries.filter((e) => e.isDir)) {
          const tok = normalizeToken(d.name);
          if (tokens.some((cand) => tok === cand || tok.includes(cand) || cand.includes(tok))) {
            const sub = await listFiles(`${csgoRoot}/cfg/${d.name}`);
            for (const f of sub) {
              if (!f.isDir && isEditablePluginConfig(f.name)) {
                collected.push(`${csgoRoot}/cfg/${d.name}/${f.name}`);
              }
            }
          }
        }
        collected.sort((a, b) => a.localeCompare(b));
        setMatchedFiles(collected);
        setSelectedFile((prev) => (prev && collected.includes(prev) ? prev : (collected[0] || "")));
      } else {
        const baseDir = (addon.targetDir || "plugins").replace(/^\/+|\/+$/g, "") || "plugins";
        const res = await listDirIfPresent(listFiles, baseDir);
        const pkgTok = normalizeToken(addon.package);
        const titleTok = normalizeToken(displayTitle);
        const dirCands = res.entries.filter((e) => e.isDir);
        const matchedDir = dirCands.find((d) => {
          const tok = normalizeToken(d.name);
          if (!tok) return false;
          if (pkgTok && (tok === pkgTok || tok.includes(pkgTok) || pkgTok.includes(tok))) return true;
          if (titleTok.length >= 3 && (tok === titleTok || tok.includes(titleTok) || titleTok.includes(tok))) {
            return true;
          }
          return false;
        });
        const collected: string[] = [];
        if (matchedDir) {
          const subPath = `${baseDir}/${matchedDir.name}`;
          const subEntries = await listFiles(subPath);
          for (const e of subEntries) {
            if (!e.isDir && isEditablePluginConfig(e.name)) {
              collected.push(`${subPath}/${e.name}`);
            }
          }
        }
        collected.sort((a, b) => a.localeCompare(b));
        setMatchedFiles(collected);
        setSelectedFile((prev) => (prev && collected.includes(prev) ? prev : (collected[0] || "")));
      }
    } catch (err) {
      setFilesError(err instanceof Error ? err.message : "读取配置文件列表失败");
      setMatchedFiles([]);
      setSelectedFile("");
    } finally {
      setFilesLoading(false);
    }
  }, [open, addon, isFileMode, isTMod, isBepInEx, isOxide, isSML, isCS2Plugin, listFiles, modIds, displayTitle, artifact?.status?.descriptor?.fileName]);

  useEffect(() => {
    void scanWorkspaceConfigs();
  }, [scanWorkspaceConfigs]);

  const isJsonFormTarget = useMemo(
    () =>
      isTMod ||
      ((isOxide || isCS2Plugin) && selectedFile.toLowerCase().endsWith(".json")) ||
      (isSML &&
        (selectedFile.toLowerCase().endsWith(".json") ||
          (fileSavedContent !== "" && parsePlainJsonObject(fileSavedContent) !== null))),
    [fileSavedContent, isCS2Plugin, isOxide, isSML, isTMod, selectedFile],
  );

  useEffect(() => {
    if (!open || !isFileMode || !selectedFile) {
      setFileContent("");
      setFileSavedContent("");
      setFileParseError(null);
      return;
    }
    let cancelled = false;
    setFileLoading(true);
    setFileParseError(null);
    void readFile(selectedFile)
      .then((text) => {
        if (cancelled) return;
        setFileContent(text);
        setFileSavedContent(text);
        const parsed = (isJsonFormTarget || isSML) ? parsePlainJsonObject(text) : null;
        const primitiveCount = parsed
          ? Object.values(parsed).filter(
              (v) => typeof v === "boolean" || typeof v === "number" || typeof v === "string",
            ).length
          : 0;
        setFileEditView((isJsonFormTarget || (isSML && parsed !== null)) && primitiveCount > 0 ? "form" : "json");
      })
      .catch((err) => {
        if (cancelled) return;
        setFilesError(err instanceof Error ? err.message : "读取文件内容失败");
      })
      .finally(() => {
        if (!cancelled) setFileLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, isFileMode, isSML, selectedFile, readFile, isJsonFormTarget]);

  const parsedJsonObj = useMemo(
    () => (isJsonFormTarget ? parsePlainJsonObject(fileContent) : null),
    [fileContent, isJsonFormTarget],
  );

  const primitiveJsonKeys = useMemo(() => {
    if (!parsedJsonObj) return [];
    return Object.entries(parsedJsonObj).filter(
      ([, v]) => typeof v === "boolean" || typeof v === "number" || typeof v === "string",
    ) as Array<[string, boolean | number | string]>;
  }, [parsedJsonObj]);

  const handleJsonPrimitiveChange = useCallback(
    (key: string, nextVal: boolean | number | string) => {
      if (!parsedJsonObj) return;
      const nextObj = { ...parsedJsonObj, [key]: nextVal };
      setFileContent(`${JSON.stringify(nextObj, null, 2)}\n`);
      setFileParseError(null);
    },
    [parsedJsonObj],
  );

  const handleSaveFile = useCallback(async () => {
    if (!selectedFile || fileSaving) return;
    if (isJsonFormTarget && selectedFile.toLowerCase().endsWith(".json")) {
      if (!parsePlainJsonObject(fileContent)) {
        try {
          JSON.parse(fileContent);
        } catch (err) {
          setFileParseError(err instanceof Error ? `JSON 格式无效: ${err.message}` : "JSON 格式无效");
          return;
        }
      }
    }
    setFileSaving(true);
    setFileParseError(null);
    try {
      await writeFile(selectedFile, fileContent);
      setFileSavedContent(fileContent);
      toast("success", running ? "已保存配置文件，重启服务器后生效" : "已保存配置文件");
      onClose();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "保存配置文件失败");
    } finally {
      setFileSaving(false);
    }
  }, [fileContent, fileSaving, isJsonFormTarget, onClose, running, selectedFile, toast, writeFile]);

  const handleCreateTModFile = useCallback(async () => {
    if (creatingBusy) return;
    const filename = sanitizeJsonFilename(createName || defaultTModFilename(effectiveModIds, displayTitle));
    const fullPath = `SaveData/ModConfigs/${filename}`;
    setCreatingBusy(true);
    try {
      await ensureDirTree(listFiles, mkdirPath, "SaveData/ModConfigs");
      if (!matchedFiles.includes(fullPath)) {
        await writeFile(fullPath, "{}\n");
      }
      setCreatingOpen(false);
      await scanWorkspaceConfigs();
      setSelectedFile(fullPath);
      setFileEditView("json");
      toast("success", `已创建配置文件 ${filename}`);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "创建配置文件失败");
    } finally {
      setCreatingBusy(false);
    }
  }, [
    createName,
    creatingBusy,
    displayTitle,
    effectiveModIds,
    listFiles,
    matchedFiles,
    mkdirPath,
    scanWorkspaceConfigs,
    toast,
    writeFile,
  ]);

  if (!addon) return null;

  const isSchemaMode = schema.length > 0;
  const fileDirty = fileContent !== fileSavedContent;

  const subheaderNode = isSchemaMode ? (
    schema.length > 8 || schemaModifiedCount > 0 ? (
      <div className="flex flex-wrap items-center justify-between gap-2">
        {schema.length > 8 ? (
          <div className="relative min-w-[200px] flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8A90A6]" />
            <Input
              value={schemaFilter}
              onChange={(e) => setSchemaFilter(e.target.value)}
              placeholder="筛选配置项..."
              className="h-9 pl-8 text-xs"
            />
          </div>
        ) : (
          <span className="text-xs text-[#5B6075]">共 {schema.length} 项可配置参数</span>
        )}
        <div className="flex items-center gap-2">
          {schemaModifiedCount > 0 && (
            <span className="text-xs font-medium text-[#2B3AE8]">已修改 {schemaModifiedCount} 项</span>
          )}
          <Button
            size="sm"
            variant="ghost"
            disabled={schemaModifiedCount === 0 || savingSchema}
            onClick={handleResetAllSchema}
          >
            恢复默认
          </Button>
        </div>
      </div>
    ) : undefined
  ) : isFileMode && (matchedFiles.length > 0 || creatingOpen) ? (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          {matchedFiles.length > 1 ? (
            <Select
              value={selectedFile}
              onChange={(e) => setSelectedFile(e.target.value)}
              aria-label="选择配置文件"
              className="h-9 max-w-xs text-xs"
            >
              {matchedFiles.map((p) => (
                <option key={p} value={p}>
                  {p.split("/").slice(-1)[0]}
                </option>
              ))}
            </Select>
          ) : matchedFiles.length === 1 ? (
            <Mono className="truncate text-xs text-[#3D4258]">{matchedFiles[0]}</Mono>
          ) : (
            <span className="text-xs text-[#5B6075]">新建配置文件</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {isJsonFormTarget && selectedFile && !creatingOpen && (
            <Segmented
              label="编辑视图"
              value={fileEditView}
              onChange={setFileEditView}
              items={[
                { value: "form", label: "表单视图" },
                { value: "json", label: "JSON 源码" },
              ]}
            />
          )}
          {!creatingOpen && (
            <Button
              size="sm"
              variant="ghost"
              icon={<RefreshCw className={`h-3.5 w-3.5 ${filesLoading ? "animate-spin" : ""}`} />}
              disabled={filesLoading}
              onClick={() => void scanWorkspaceConfigs()}
            >
              重新读取目录
            </Button>
          )}
          {isTMod && !creatingOpen && (
            <Button
              size="sm"
              variant="ghost"
              icon={<FilePlus2 className="h-3.5 w-3.5" />}
              onClick={() => {
                setCreateName(defaultTModFilename(effectiveModIds, displayTitle));
                setCreatingOpen(true);
              }}
            >
              新建
            </Button>
          )}
        </div>
      </div>

      {creatingOpen && (
        <div className="flex flex-wrap items-center gap-2 rounded-xl bg-[#F7F8FC] px-3 py-2.5">
          <Input
            value={createName}
            onChange={(e) => setCreateName(e.target.value)}
            placeholder="ModName_ServerConfig.json"
            className="h-9 min-w-[200px] flex-1 font-mono text-xs"
          />
          <Button
            size="sm"
            variant="primary"
            loading={creatingBusy}
            onClick={() => void handleCreateTModFile()}
          >
            创建文件
          </Button>
          <Button size="sm" variant="ghost" disabled={creatingBusy} onClick={() => setCreatingOpen(false)}>
            取消
          </Button>
        </div>
      )}
    </div>
  ) : undefined;

  const footerNode = isSchemaMode ? (
    <>
      <Button variant="ghost" onClick={onClose} disabled={savingSchema}>
        取消
      </Button>
      <Button variant="primary" loading={savingSchema} onClick={() => void handleSaveSchema()}>
        保存配置
      </Button>
    </>
  ) : isFileMode && selectedFile && !filesError ? (
    <>
      <Button variant="ghost" onClick={onClose} disabled={fileSaving}>
        关闭
      </Button>
      <Button
        variant="primary"
        loading={fileSaving}
        disabled={fileLoading || !fileDirty}
        onClick={() => void handleSaveFile()}
      >
        保存文件
      </Button>
    </>
  ) : (
    <Button variant="ghost" onClick={onClose}>
      关闭
    </Button>
  );

  return (
    <>
      <Modal
      open={open}
      onClose={onClose}
      title={`${displayTitle} · 配置`}
      description={
        isSchemaMode
          ? running
            ? "修改后将写入模组配置，重启服务器即可应用最新选项"
            : "修改后将在下次启动服务器时自动写入模组配置"
          : isFileMode
            ? `直接编辑实例工作区中的${noun}配置文件`
            : undefined
      }
      subheader={subheaderNode}
      footer={footerNode}
      width="max-w-2xl"
    >
      {isSchemaMode ? (
        visibleSchemaFields.length === 0 ? (
          <div className="py-10 text-center text-sm text-[#5B6075]">没有匹配“{schemaFilter}”的配置项</div>
        ) : (
          <div className="divide-y divide-[#EEF0F7]">
            {visibleSchemaFields.map((field) => {
              const current = schemaDraft[field.name] ?? "";
              const def = field.default ?? "";
              const isOverridden = current !== def;
              return (
                <div
                  key={field.name}
                  className="flex flex-col justify-between gap-3 py-3.5 first:pt-1 last:pb-1 sm:flex-row sm:items-center"
                >
                  <div className="min-w-0 flex-1 space-y-0.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium text-[#14161F]">{field.label || field.name}</span>
                      {isOverridden && <Badge tone="accent">已修改</Badge>}
                    </div>
                    {field.description && (
                      <p className="text-xs leading-relaxed text-[#5B6075]">{field.description}</p>
                    )}
                    {field.label && field.label !== field.name && (
                      <Mono className="inline-block text-[11px] text-[#8A90A6]">{field.name}</Mono>
                    )}
                  </div>

                  <div className="flex shrink-0 items-center gap-1.5">
                    {field.type === "boolean" ? (
                      <Switch
                        checked={current === "true"}
                        onChange={(checked) =>
                          setSchemaDraft((prev) => ({ ...prev, [field.name]: checked ? "true" : "false" }))
                        }
                        label={field.label || field.name}
                      />
                    ) : field.type === "select" ? (
                      <Select
                        value={current}
                        onChange={(e) =>
                          setSchemaDraft((prev) => ({ ...prev, [field.name]: e.target.value }))
                        }
                        aria-label={field.label || field.name}
                        className="h-9 w-48 text-xs"
                      >
                        {current &&
                          !(field.options || []).some((opt) => opt.value === current) && (
                            <option value={current}>{current}</option>
                          )}
                        {(field.options || []).map((opt) => (
                          <option key={opt.value} value={opt.value}>
                            {opt.label || opt.value}
                          </option>
                        ))}
                      </Select>
                    ) : field.type === "number" ? (
                      <Input
                        type="number"
                        value={current}
                        min={field.min}
                        max={field.max}
                        step={field.step ?? "any"}
                        onChange={(e) =>
                          setSchemaDraft((prev) => ({ ...prev, [field.name]: e.target.value }))
                        }
                        className="h-9 w-36 text-xs tabular-nums"
                      />
                    ) : (
                      <Input
                        value={current}
                        onChange={(e) =>
                          setSchemaDraft((prev) => ({ ...prev, [field.name]: e.target.value }))
                        }
                        className="h-9 w-52 text-xs"
                      />
                    )}

                    <IconButton
                      label={`恢复 ${field.label || field.name} 默认值`}
                      disabled={!isOverridden}
                      onClick={() =>
                        setSchemaDraft((prev) => ({ ...prev, [field.name]: def }))
                      }
                      className={isOverridden ? "opacity-100" : "pointer-events-none opacity-0"}
                    >
                      <RotateCcw className="h-3.5 w-3.5" />
                    </IconButton>
                  </div>
                </div>
              );
            })}
          </div>
        )
      ) : isFileMode ? (
        filesLoading ? (
          <div className="space-y-3 py-2">
            <Skeleton className="h-10 w-full rounded-xl" />
            <Skeleton className="h-44 w-full rounded-xl" />
          </div>
        ) : filesError ? (
          <div className="rounded-xl bg-[#FEF3F2] px-4 py-3 text-sm text-[#B42318]">{filesError}</div>
        ) : matchedFiles.length === 0 ? (
          <EmptyState
            image={null}
            title={`暂未生成${noun}配置文件`}
            body={
              isTMod
                ? "tModLoader 仅在模组配置与默认值不同或游戏内保存过配置后，才会在 SaveData/ModConfigs 目录生成 JSON 配置文件。你也可以手动新建配置文件。"
                : isOxide || isBepInEx
                  ? `首次启动服务器加载该${noun}后，将在 ${(addon.targetDir || (isOxide ? "oxide" : "BepInEx")).replace(/^\/+|\/+$/g, "")}/config 目录自动生成对应的配置文件。`
                  : isSML
                    ? "首次启动服务器加载该模组后，若模组提供配置将在 FactoryGame/Configs 目录自动生成对应配置文件。"
                    : isCS2Plugin
                      ? `首次启动服务器加载该插件后，将在 ${(addon.targetDir || "game/csgo/addons").replace(/^\/+|\/+$/g, "")}/counterstrikesharp/configs/plugins 或 game/csgo/cfg 目录生成对应配置文件。`
                      : `首次启动服务器加载该${noun}后，将在 ${(addon.targetDir || "plugins").replace(/^\/+|\/+$/g, "") || "plugins"} 目录自动生成对应的配置文件。`
            }
            action={
              isTMod ? (
                <div className="flex flex-wrap items-center justify-center gap-2">
                  <Button
                    size="sm"
                    variant="primary"
                    icon={<FilePlus2 className="h-4 w-4" />}
                    onClick={() => {
                      setCreateName(defaultTModFilename(effectiveModIds, displayTitle));
                      setCreatingOpen(true);
                    }}
                  >
                    新建配置文件
                  </Button>
                  <Button
                    size="sm"
                    variant="secondary"
                    icon={<RefreshCw className={`h-4 w-4 ${filesLoading ? "animate-spin" : ""}`} />}
                    loading={filesLoading}
                    onClick={() => void scanWorkspaceConfigs()}
                  >
                    重新读取目录
                  </Button>
                </div>
              ) : (
                <Button
                  size="sm"
                  variant="secondary"
                  icon={<RefreshCw className={`h-4 w-4 ${filesLoading ? "animate-spin" : ""}`} />}
                  loading={filesLoading}
                  onClick={() => void scanWorkspaceConfigs()}
                >
                  重新读取目录
                </Button>
              )
            }
          />
        ) : fileLoading ? (
          <div className="space-y-3 py-2">
            <Skeleton className="h-10 w-full rounded-xl" />
            <Skeleton className="h-52 w-full rounded-xl" />
          </div>
        ) : (
          <div className="space-y-3">
            {fuzzyNotice && (
              <p className="rounded-xl bg-[#F7F8FC] px-3.5 py-2 text-xs text-[#5B6075]">{fuzzyNotice}</p>
            )}
            {fileParseError && (
              <p className="rounded-xl bg-[#FEF3F2] px-3.5 py-2 text-xs text-[#B42318]">{fileParseError}</p>
            )}
            {isJsonFormTarget && fileEditView === "form" ? (
              primitiveJsonKeys.length > 0 ? (
                <div className="divide-y divide-[#EEF0F7]">
                  {primitiveJsonKeys.map(([key, val]) => (
                    <div
                      key={key}
                      className="flex flex-col justify-between gap-2 py-3 first:pt-1 last:pb-1 sm:flex-row sm:items-center"
                    >
                      <Mono className="min-w-0 truncate text-xs text-[#14161F]">{key}</Mono>
                      <div className="shrink-0">
                        {typeof val === "boolean" ? (
                          <Switch
                            checked={val}
                            onChange={(checked) => handleJsonPrimitiveChange(key, checked)}
                            label={key}
                          />
                        ) : typeof val === "number" ? (
                          <Input
                            type="number"
                            value={String(val)}
                            step="any"
                            onChange={(e) => {
                              const raw = e.target.value;
                              if (raw === "" || raw === "-") return;
                              const num = Number(raw);
                              if (!Number.isNaN(num)) handleJsonPrimitiveChange(key, num);
                            }}
                            className="h-9 w-40 text-xs tabular-nums"
                          />
                        ) : (
                          <Input
                            value={val}
                            onChange={(e) => handleJsonPrimitiveChange(key, e.target.value)}
                            className="h-9 w-56 text-xs"
                          />
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <EmptyState
                  image={null}
                  title="暂无可渲染的基础键值"
                  body="当前 JSON 文件为空对象或仅包含嵌套结构，请切换到 JSON 源码视图编辑字段。"
                  action={
                    <Button size="sm" variant="secondary" onClick={() => setFileEditView("json")}>
                      切换到 JSON 源码
                    </Button>
                  }
                />
              )
            ) : (
              <Textarea
                value={fileContent}
                onChange={(e) => {
                  setFileContent(e.target.value);
                  setFileParseError(null);
                }}
                spellCheck={false}
                className="min-h-[300px] font-mono text-[13px] leading-relaxed"
              />
            )}
          </div>
        )
      ) : (
        <EmptyState
          image={null}
          title={`该${noun}未声明可配置选项`}
          body={`当前${noun}包未包含可调节的参数定义，启用后即可直接生效。`}
          action={
            isAdmin && artifact && onResyncArtifact && Boolean(artifact.status?.descriptor?.digest) ? (
              <Button
                size="sm"
                variant="secondary"
                loading={retrying}
                onClick={() => setConfirmExtractOpen(true)}
              >
                从缓存提取配置
              </Button>
            ) : undefined
          }
        />
      )}
      </Modal>
      <ConfirmDialog
        open={confirmExtractOpen}
        onClose={() => {
          if (!retrying) setConfirmExtractOpen(false);
        }}
        title={`确认从缓存提取${noun}配置`}
        body="将从对象存储缓存读取该模组包并重新解析配置定义；仅当上游版本发生变化时才会重新下载，是否继续？"
        confirmLabel="开始提取"
        loading={retrying}
        onConfirm={async () => {
          if (!artifact || !onResyncArtifact) return;
          await onResyncArtifact(artifact);
          setConfirmExtractOpen(false);
        }}
      />
    </>
  );
}
