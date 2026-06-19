import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./features/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        panel: {
          bg: "#070a0f",
          sidebar: "#0c1118",
          card: "#111821",
          line: "#202a36",
          green: "#59d46f",
          purple: "#a873ff",
          gold: "#e6b84a",
          red: "#ff6b6b"
        }
      }
    }
  },
  plugins: []
};

export default config;
