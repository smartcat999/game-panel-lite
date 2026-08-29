import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { BrowserFrame, Pill, SceneBackground, SceneTitle } from "../components";
import { copy, type Locale } from "../copy";
import { monoFamily, palette } from "../theme";

export const InviteScene: React.FC<{ locale: Locale }> = ({ locale }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const text = copy[locale].invite;
  const copied = frame > 95;
  const enter = spring({ frame: frame - 18, fps, config: { damping: 18, mass: 0.75 } });

  return (
    <SceneBackground>
      <SceneTitle eyebrow={text.eyebrow} title={text.title} body={text.body} />
      <BrowserFrame label={copy[locale].nav}>
        <div style={{ height: "100%", display: "grid", placeItems: "center", paddingBottom: 12 }}>
          <div
            style={{
              width: 920,
              padding: 34,
              borderRadius: 24,
              border: `1px solid ${palette.green}66`,
              background: "linear-gradient(145deg, rgba(17,29,43,.98), rgba(8,14,23,.98))",
              boxShadow: "0 30px 80px rgba(0,0,0,.38)",
              opacity: enter,
              scale: 0.94 + enter * 0.06,
            }}
          >
            <div style={{ display: "flex", alignItems: "center" }}>
              <div
                style={{
                  width: 62,
                  height: 62,
                  display: "grid",
                  placeItems: "center",
                  borderRadius: 17,
                  color: palette.green,
                  background: "rgba(88,214,116,.12)",
                  fontSize: 31,
                }}
              >
                🎮
              </div>
              <div style={{ marginLeft: 18 }}>
                <div style={{ fontSize: 27, fontWeight: 830 }}>{text.heading}</div>
                <div style={{ marginTop: 8, display: "flex", gap: 9, alignItems: "center" }}>
                  <span
                    style={{
                      width: 10,
                      height: 10,
                      borderRadius: "50%",
                      background: palette.green,
                      boxShadow: `0 0 14px ${palette.green}`,
                    }}
                  />
                  <span style={{ color: palette.green, fontSize: 17 }}>{text.online}</span>
                </div>
              </div>
              <div style={{ marginLeft: "auto" }}>
                <Pill color={palette.gold} background="rgba(232,184,75,.1)">Don't Starve Together</Pill>
              </div>
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16, marginTop: 26 }}>
              {[
                [text.address, "play.gamepanel.site:10999"],
                [text.password, "campfire"],
              ].map(([label, value]) => (
                <div
                  key={label}
                  style={{
                    padding: "18px 20px",
                    borderRadius: 14,
                    border: `1px solid ${palette.line}`,
                    background: "#070d16",
                  }}
                >
                  <div style={{ color: palette.muted, fontSize: 15 }}>{label}</div>
                  <div style={{ marginTop: 9, fontFamily: monoFamily, fontSize: 19, color: palette.text }}>
                    {value}
                  </div>
                </div>
              ))}
            </div>
            <div
              style={{
                height: 62,
                marginTop: 20,
                display: "grid",
                placeItems: "center",
                borderRadius: 13,
                color: copied ? palette.green : "#06110a",
                background: copied ? "rgba(88,214,116,.1)" : palette.green,
                border: copied ? `1px solid ${palette.green}77` : undefined,
                fontSize: 19,
                fontWeight: 820,
                scale: copied ? interpolate(frame, [95, 102, 110], [1, 1.035, 1], {
                  extrapolateLeft: "clamp",
                  extrapolateRight: "clamp",
                }) : 1,
              }}
            >
              {copied ? `✓ ${text.copied}` : `↗ ${text.copy}`}
            </div>
          </div>
        </div>
      </BrowserFrame>
    </SceneBackground>
  );
};
