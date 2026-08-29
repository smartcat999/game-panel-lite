import { CanvasImage, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { BrowserFrame, Pill, SceneBackground, SceneTitle } from "../components";
import { copy, type Locale } from "../copy";
import { palette } from "../theme";

const games = [
  { name: "Terraria", image: "games/terraria.jpg", accent: palette.green, zh: "原版 · tModLoader", en: "Vanilla · tModLoader" },
  { name: "Don't Starve Together", image: "games/dst.jpg", accent: palette.gold, zh: "地面 · 洞穴", en: "Forest · Caves" },
  { name: "Palworld", image: "games/palworld.jpg", accent: palette.cyan, zh: "多人联机世界", en: "Multiplayer world" },
  { name: "Minecraft", image: "games/minecraft.jpg", accent: palette.purple, zh: "原版 · Paper · Fabric", en: "Vanilla · Paper · Fabric" },
];

export const GameLibraryScene: React.FC<{ locale: Locale }> = ({ locale }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const text = copy[locale].library;

  return (
    <SceneBackground>
      <SceneTitle eyebrow={text.eyebrow} title={text.title} body={text.body} />
      <BrowserFrame label={copy[locale].nav}>
        <div style={{ padding: "34px 40px" }}>
          <div style={{ display: "flex", alignItems: "center" }}>
            <div style={{ fontSize: 26, fontWeight: 820 }}>{text.create}</div>
            <Pill color={palette.cyan} background="rgba(49,184,230,.1)">
              1 / 3
            </Pill>
            <div style={{ marginLeft: "auto", color: palette.muted, fontSize: 17 }}>
              {text.ready}
            </div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 20, marginTop: 28 }}>
            {games.map((game, index) => {
              const enter = spring({
                frame: frame - 18 - index * 8,
                fps,
                config: { damping: 18, mass: 0.7 },
              });
              const selected = Math.floor((frame / 34) % games.length) === index;
              return (
                <div
                  key={game.name}
                  style={{
                    height: 360,
                    overflow: "hidden",
                    position: "relative",
                    borderRadius: 20,
                    border: `2px solid ${selected ? game.accent : palette.line}`,
                    background: palette.panel,
                    opacity: enter,
                    scale: 0.94 + enter * 0.06,
                    translate: `0 ${(1 - enter) * 30}px`,
                    boxShadow: selected ? `0 18px 48px ${game.accent}22` : undefined,
                  }}
                >
                  <CanvasImage
                    src={staticFile(game.image)}
                    style={{ width: "100%", height: 258, objectFit: "cover" }}
                  />
                  <div
                    style={{
                      position: "absolute",
                      left: 0,
                      right: 0,
                      bottom: 0,
                      height: 130,
                      padding: "48px 20px 0",
                      background: "linear-gradient(transparent, rgba(8,14,23,.98) 42%)",
                    }}
                  >
                    <div style={{ fontSize: game.name.length > 15 ? 20 : 24, fontWeight: 820 }}>
                      {game.name}
                    </div>
                    <div style={{ marginTop: 9, color: selected ? game.accent : palette.muted, fontSize: 16 }}>
                      {selected ? `✓ ${text.ready}` : game[locale]}
                    </div>
                  </div>
                  <div
                    style={{
                      position: "absolute",
                      inset: 0,
                      opacity: interpolate(frame, [0, 160], [0.1, 0.28], {
                        extrapolateLeft: "clamp",
                        extrapolateRight: "clamp",
                      }),
                      background: selected ? `linear-gradient(135deg, transparent 55%, ${game.accent}55)` : undefined,
                    }}
                  />
                </div>
              );
            })}
          </div>
        </div>
      </BrowserFrame>
    </SceneBackground>
  );
};
