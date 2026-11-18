import type { CSSProperties, ReactNode } from "react";
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { fontFamily, palette } from "./theme";

export const SceneBackground: React.FC<{ children: ReactNode }> = ({ children }) => {
  const frame = useCurrentFrame();
  const drift = interpolate(frame, [0, 180], [0, 36], { extrapolateRight: "extend" });

  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        overflow: "hidden",
        color: palette.text,
        fontFamily,
        background: palette.background,
      }}
    >
      <div
        style={{
          position: "absolute",
          inset: -80,
          opacity: 0.28,
          backgroundImage:
            "linear-gradient(rgba(70,94,126,.16) 1px, transparent 1px), linear-gradient(90deg, rgba(70,94,126,.16) 1px, transparent 1px)",
          backgroundSize: "64px 64px",
          transform: `translate(${-drift}px, ${drift * 0.35}px)`,
        }}
      />
      <div
        style={{
          position: "absolute",
          width: 760,
          height: 760,
          right: -260,
          top: -330,
          borderRadius: "50%",
          background: "rgba(52, 211, 103, .10)",
          filter: "blur(90px)",
        }}
      />
      {children}
    </div>
  );
};

export const BrandMark: React.FC<{ compact?: boolean }> = ({ compact = false }) => (
  <div style={{ display: "flex", alignItems: "center", gap: compact ? 16 : 24 }}>
    <div
      style={{
        width: compact ? 54 : 84,
        height: compact ? 54 : 84,
        borderRadius: compact ? 16 : 24,
        display: "grid",
        placeItems: "center",
        border: `2px solid ${palette.green}`,
        color: palette.green,
        background: "rgba(88, 214, 116, .08)",
        fontSize: compact ? 28 : 44,
        lineHeight: 1,
      }}
    >
      ◉
    </div>
    <div style={{ fontWeight: 840, fontSize: compact ? 34 : 62, letterSpacing: -1.8 }}>
      GamePanel <span style={{ color: palette.green }}>Lite</span>
    </div>
  </div>
);

export const SceneTitle: React.FC<{ eyebrow: string; title: string; body: string }> = ({
  eyebrow,
  title,
  body,
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const enter = spring({ frame, fps, config: { damping: 18, mass: 0.7 } });

  return (
    <div
      style={{
        opacity: enter,
        transform: `translateY(${(1 - enter) * 28}px)`,
        width: 1640,
        margin: "72px auto 0",
      }}
    >
      <div
        style={{
          color: palette.green,
          fontWeight: 750,
          fontSize: 24,
          letterSpacing: 4,
          textTransform: "uppercase",
          marginBottom: 16,
        }}
      >
        {eyebrow}
      </div>
      <div style={{ fontSize: 76, fontWeight: 840, letterSpacing: -3.6, lineHeight: 1.05 }}>
        {title}
      </div>
      <div style={{ marginTop: 20, fontSize: 30, color: palette.muted, lineHeight: 1.45 }}>
        {body}
      </div>
    </div>
  );
};

export const BrowserFrame: React.FC<{
  children: ReactNode;
  style?: CSSProperties;
  label?: string;
}> = ({
  children,
  style,
  label = "Ready to play · 4 games",
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const enter = spring({ frame: frame - 12, fps, config: { damping: 20, mass: 0.85 } });
  return (
    <div
      style={{
        position: "absolute",
        left: 140,
        right: 140,
        bottom: 60,
        height: 590,
        overflow: "hidden",
        borderRadius: 28,
        border: `1px solid ${palette.line}`,
        background: "#080e17",
        boxShadow: "0 42px 100px rgba(0,0,0,.48)",
        opacity: enter,
        transform: `translateY(${(1 - enter) * 46}px) scale(${0.97 + enter * 0.03})`,
        ...style,
      }}
    >
      <div
        style={{
          height: 74,
          display: "flex",
          alignItems: "center",
          padding: "0 28px",
          borderBottom: `1px solid ${palette.lineSoft}`,
          background: "#0a111b",
        }}
      >
        <BrandMark compact />
        <div
          style={{
            marginLeft: 28,
            padding: "11px 18px",
            borderRadius: 99,
            border: `1px solid rgba(49,184,230,.42)`,
            color: palette.cyan,
            fontSize: 19,
            letterSpacing: 1.3,
          }}
        >
          {label}
        </div>
        <div style={{ marginLeft: "auto", display: "flex", gap: 13 }}>
          {["◌", "▣", "⌁", "⚙"].map((item) => (
            <div
              key={item}
              style={{
                width: 42,
                height: 42,
                display: "grid",
                placeItems: "center",
                borderRadius: 12,
                color: palette.muted,
                background: palette.panel,
              }}
            >
              {item}
            </div>
          ))}
        </div>
      </div>
      {children}
    </div>
  );
};

export const Pill: React.FC<{
  children: ReactNode;
  color?: string;
  background?: string;
}> = ({ children, color = palette.green, background = "rgba(88,214,116,.12)" }) => (
  <span
    style={{
      padding: "6px 12px",
      borderRadius: 8,
      fontWeight: 720,
      fontSize: 17,
      color,
      background,
      whiteSpace: "nowrap",
    }}
  >
    {children}
  </span>
);
