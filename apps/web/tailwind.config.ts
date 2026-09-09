import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./features/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        panel: {
          bg: "#f8fafc",
          surface: "#ffffff",
          sidebar: "#ffffff",
          card: "#ffffff",
          line: "#e2e8f0",
          border: "rgba(226, 232, 240, 0.85)",
          text: "#1e293b",
          muted: "#64748b",
          green: "#10b981",
          purple: "#8b5cf6",
          gold: "#f59e0b",
          red: "#ef4444"
        }
      },
      boxShadow: {
        subtle: "0 1px 2px 0 rgba(15, 23, 42, 0.04), 0 1px 6px -1px rgba(15, 23, 42, 0.02)"
      }
    }
  },
  plugins: []
};

export default config;
