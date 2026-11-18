import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { BrandMark, SceneBackground } from "../components";
import { copy, type Locale } from "../copy";
import { palette } from "../theme";

export const IntroScene: React.FC<{ locale: Locale }> = ({ locale }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const text = copy[locale].intro;
  const reveal = spring({ frame, fps, config: { damping: 16, mass: 0.8 } });
  const line = interpolate(frame, [24, 64], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  return (
    <SceneBackground>
      <div
        style={{
          position: "absolute",
          inset: 0,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          transform: `scale(${0.92 + reveal * 0.08})`,
          opacity: reveal,
        }}
      >
        <BrandMark />
        <div
          style={{
            width: 760 * line,
            height: 2,
            margin: "42px 0 34px",
            background: `linear-gradient(90deg, transparent, ${palette.green}, transparent)`,
          }}
        />
        <div
          style={{
            maxWidth: 1450,
            textAlign: "center",
            fontSize: locale === "zh" ? 58 : 54,
            fontWeight: 790,
            letterSpacing: locale === "zh" ? 1.4 : -1.6,
            lineHeight: 1.18,
          }}
        >
          {text.title}
        </div>
        <div
          style={{
            maxWidth: 1220,
            marginTop: 22,
            textAlign: "center",
            color: palette.muted,
            fontSize: 26,
            lineHeight: 1.45,
          }}
        >
          {text.body}
        </div>
        <div style={{ marginTop: 24, color: palette.green, fontSize: 22, letterSpacing: 2 }}>
          {text.kicker}
        </div>
      </div>
    </SceneBackground>
  );
};
