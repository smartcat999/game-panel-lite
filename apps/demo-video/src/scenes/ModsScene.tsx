import { spring, useCurrentFrame, useVideoConfig } from "remotion";
import { BrowserFrame, Pill, SceneBackground, SceneTitle } from "../components";
import { copy, type Locale } from "../copy";
import { palette } from "../theme";

const mods = [
  ["更多物品堆叠 / More Items Stack", "57 KB", "仅服务端"],
  ["Pond OceanTree!", "460 KB", "服务端与客户端"],
  ["Epic Healthbar", "830 KB", "服务端与客户端"],
  ["Fast Travel", "47 KB", "仅服务端"],
];

const times = {
  zh: ["刚刚", "16 小时前", "19 天前", "21 天前"],
  en: ["Just now", "16 hours ago", "19 days ago", "21 days ago"],
};

export const ModsScene: React.FC<{ locale: Locale }> = ({ locale }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const text = copy[locale].mods;
  return (
    <SceneBackground>
      <SceneTitle
        eyebrow={text.eyebrow}
        title={text.title}
        body={text.body}
      />
      <BrowserFrame label={copy[locale].nav}>
        <div style={{ padding: "30px 42px" }}>
          <div style={{ display: "flex", alignItems: "center" }}>
            <div>
              <div style={{ fontSize: 26, fontWeight: 820 }}>{text.heading}</div>
              <div style={{ marginTop: 8, color: palette.muted, fontSize: 17 }}>
                {text.hint}
              </div>
            </div>
            <div style={{ marginLeft: "auto", display: "flex", gap: 10 }}>
              <Pill>{text.library}</Pill>
              <Pill color={palette.muted} background={palette.panelRaised}>{text.packs}</Pill>
            </div>
          </div>
          <div
            style={{
              marginTop: 24,
              border: `1px solid ${palette.line}`,
              borderRadius: 18,
              overflow: "hidden",
            }}
          >
            {mods.map(([name, meta, side], index) => {
              const enter = spring({
                frame: frame - 20 - index * 9,
                fps,
                config: { damping: 18, mass: 0.6 },
              });
              const active = frame > 62 + index * 10;
              const focus = active && index === Math.floor(((frame - 62) / 28) % 4);
              return (
                <div
                  key={name}
                  style={{
                    height: 90,
                    display: "flex",
                    alignItems: "center",
                    gap: 18,
                    padding: "0 22px",
                    borderBottom: index === mods.length - 1 ? undefined : `1px solid ${palette.lineSoft}`,
                    opacity: enter,
                    transform: `translateX(${(1 - enter) * 34}px)`,
                    background: focus ? "rgba(88,214,116,.055)" : "transparent",
                  }}
                >
                  <div
                    style={{
                      width: 48,
                      height: 48,
                      borderRadius: 13,
                      display: "grid",
                      placeItems: "center",
                      color: palette.green,
                      background: "rgba(88,214,116,.09)",
                      border: "1px solid rgba(88,214,116,.32)",
                      fontSize: 22,
                    }}
                  >
                    {active ? "✓" : "◇"}
                  </div>
                  <div>
                    <div style={{ fontSize: 21, fontWeight: 760 }}>{name}</div>
                    <div style={{ marginTop: 7, color: palette.muted, fontSize: 16 }}>
                      {meta} · {times[locale][index]}
                    </div>
                  </div>
                  <div style={{ marginLeft: "auto", display: "flex", gap: 12, alignItems: "center" }}>
                    <Pill
                      color={side === "仅服务端" ? palette.green : palette.gold}
                      background={side === "仅服务端" ? "rgba(88,214,116,.1)" : "rgba(232,184,75,.1)"}
                    >
                      {side === "仅服务端" ? text.serverOnly : text.both}
                    </Pill>
                    <div
                      style={{
                        width: 116,
                        padding: "10px 0",
                        textAlign: "center",
                        borderRadius: 10,
                        fontSize: 17,
                        fontWeight: 750,
                        color: active ? palette.green : palette.text,
                        border: `1px solid ${active ? palette.green + "66" : palette.line}`,
                        background: active ? "rgba(88,214,116,.08)" : palette.panel,
                      }}
                    >
                      {active ? text.installed : text.install}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </BrowserFrame>
    </SceneBackground>
  );
};
