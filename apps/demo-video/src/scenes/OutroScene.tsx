import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { BrandMark, SceneBackground } from "../components";
import { copy, type Locale } from "../copy";
import { palette } from "../theme";

export const OutroScene: React.FC<{ locale: Locale }> = ({ locale }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const text = copy[locale].outro;
  const enter = spring({ frame, fps, config: { damping: 18, mass: 0.8 } });
  const glow = interpolate(Math.sin(frame / 14), [-1, 1], [0.24, 0.55]);
  return (
    <SceneBackground>
      <div
        style={{
          position: "absolute",
          inset: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexDirection: "column",
          opacity: enter,
          transform: `translateY(${(1 - enter) * 28}px)`,
        }}
      >
        <BrandMark />
        <div style={{ marginTop: 46, fontSize: 54, fontWeight: 790, letterSpacing: -0.6 }}>
          {text.title}
        </div>
        <div style={{ marginTop: 16, color: palette.muted, fontSize: 27 }}>
          {text.body}
        </div>
        <div
          style={{
            marginTop: 38,
            padding: "16px 28px",
            borderRadius: 14,
            border: `1px solid rgba(88,214,116,${glow})`,
            background: "rgba(88,214,116,.07)",
            color: palette.green,
            fontSize: 25,
            fontWeight: 720,
            letterSpacing: 0.6,
          }}
        >
          github.com/smartcat999/game-panel-lite
        </div>
      </div>
    </SceneBackground>
  );
};
