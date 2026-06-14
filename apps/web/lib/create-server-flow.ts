import type { TerrariaConfig } from "@gamepanel-lite/shared";
import { assignWorld, createServer, importWorld, uploadMod } from "./api";
import type { Server, World } from "./types";

type CreateMode = "vanilla" | "tmodloader";

type CreateServerWithAssetsDeps = {
  createServer: typeof createServer;
  importWorld: typeof importWorld;
  assignWorld: typeof assignWorld;
  uploadMod: typeof uploadMod;
};

export type CreateServerWithAssetsInput = {
  config: TerrariaConfig;
  deps?: CreateServerWithAssetsDeps;
  mode: CreateMode;
  modFiles: File[];
  worldFile: File | null;
};

export type CreatedServerWithAssets = {
  assignedWorld?: World;
  server: Server;
};

const defaultDeps: CreateServerWithAssetsDeps = {
  assignWorld,
  createServer,
  importWorld,
  uploadMod
};

export async function createTerrariaServerWithAssets({
  config,
  deps = defaultDeps,
  mode,
  modFiles,
  worldFile
}: CreateServerWithAssetsInput): Promise<CreatedServerWithAssets> {
  let server = await deps.createServer({
    name: config.serverName || "Terraria Server",
    providerKey: mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla",
    config
  });

  let assignedWorld: World | undefined;
  if (worldFile) {
    const importedWorld = await deps.importWorld(worldFile, server.id);
    assignedWorld = await deps.assignWorld(importedWorld.id, server.id);
    server = {
      ...server,
      config: {
        ...server.config,
        worldName: assignedWorld.name
      },
      world: assignedWorld.name
    };
  }

  if (mode === "tmodloader" && modFiles.length > 0) {
    await Promise.all(modFiles.map((file) => deps.uploadMod(server.id, file)));
  }

  return { assignedWorld, server };
}
