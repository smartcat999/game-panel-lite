import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { BrowserFrame, Pill, SceneBackground, SceneTitle } from "../components";
import { copy, type Locale } from "../copy";
import { palette } from "../theme";

export const BackupScene: React.FC<{ locale: Locale }> = ({ locale }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const text = copy[locale].backups;
  const backups = [
    { name: text.auto, time: text.now, size: "246 MB", color: palette.green },
    { name: text.manual, time: text.yesterday, size: "241 MB", color: palette.cyan },
    { name: text.beforeUpdate, time: "2026-08-28 20:16", size: "238 MB", color: palette.gold },
  ];

  return (
    <SceneBackground>
      <SceneTitle eyebrow={text.eyebrow} title={text.title} body={text.body} />
      <BrowserFrame label={copy[locale].nav}>
        <div style={{ padding: "30px 40px" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <div style={{ fontSize: 26, fontWeight: 820 }}>{text.eyebrow}</div>
            <Pill>3</Pill>
            <div style={{ marginLeft: "auto", display: "flex", gap: 12 }}>
              {[text.save, text.backup].map((label, index) => (
                <div
                  key={label}
                  style={{
                    padding: "12px 18px",
                    borderRadius: 11,
                    fontSize: 17,
                    fontWeight: 760,
                    color: index === 1 ? "#06110a" : palette.text,
                    border: index === 1 ? undefined : `1px solid ${palette.line}`,
                    background: index === 1 ? palette.green : palette.panel,
                  }}
                >
                  {index === 0 ? "▣ " : "+ "}{label}
                </div>
              ))}
            </div>
          </div>
          <div style={{ position: "relative", marginTop: 28, paddingLeft: 38 }}>
            <div
              style={{
                position: "absolute",
                left: 14,
                top: 28,
                width: 3,
                height: interpolate(frame, [20, 118], [0, 300], {
                  extrapolateLeft: "clamp",
                  extrapolateRight: "clamp",
                }),
                borderRadius: 99,
                background: `linear-gradient(${palette.green}, ${palette.cyan}, ${palette.gold})`,
              }}
            />
            {backups.map((backup, index) => {
              const enter = spring({
                frame: frame - 18 - index * 14,
                fps,
                config: { damping: 18, mass: 0.7 },
              });
              const selected = frame > 90 && index === 0;
              return (
                <div
                  key={backup.name}
                  style={{
                    position: "relative",
                    height: 104,
                    marginBottom: 14,
                    display: "flex",
                    alignItems: "center",
                    gap: 18,
                    padding: "0 22px",
                    borderRadius: 16,
                    border: `1px solid ${selected ? backup.color + "88" : palette.line}`,
                    background: selected ? "rgba(88,214,116,.055)" : palette.panel,
                    opacity: enter,
                    translate: `${(1 - enter) * 34}px 0`,
                  }}
                >
                  <div
                    style={{
                      position: "absolute",
                      left: -35,
                      width: 18,
                      height: 18,
                      borderRadius: "50%",
                      border: `4px solid ${backup.color}`,
                      background: palette.background,
                      boxShadow: `0 0 18px ${backup.color}55`,
                    }}
                  />
                  <div
                    style={{
                      width: 52,
                      height: 52,
                      display: "grid",
                      placeItems: "center",
                      borderRadius: 14,
                      color: backup.color,
                      background: `${backup.color}14`,
                      fontSize: 24,
                    }}
                  >
                    ◴
                  </div>
                  <div>
                    <div style={{ fontSize: 21, fontWeight: 780 }}>{backup.name}</div>
                    <div style={{ marginTop: 7, color: palette.muted, fontSize: 16 }}>{backup.time}</div>
                  </div>
                  <div style={{ marginLeft: "auto", color: palette.muted, fontSize: 17 }}>{backup.size}</div>
                  <div
                    style={{
                      minWidth: 168,
                      padding: "11px 16px",
                      textAlign: "center",
                      borderRadius: 10,
                      border: `1px solid ${palette.line}`,
                      color: selected ? palette.green : palette.text,
                      fontWeight: 730,
                      fontSize: 16,
                    }}
                  >
                    {text.restore}
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
