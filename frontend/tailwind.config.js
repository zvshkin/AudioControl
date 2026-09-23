/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./src/**/*.{html,js,jsx,ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: {
          main: "#0b0d10",
          card: "rgba(20, 23, 29, 0.75)",
          hover: "rgba(28, 32, 41, 0.85)",
        },
        accent: {
          cyan: "#00f0ff",
          violet: "#7000ff",
          red: "#ff2a6d",
        },
        border: {
          subtle: "rgba(255, 255, 255, 0.08)",
          glow: "rgba(0, 240, 255, 0.25)",
        },
      },
      fontFamily: {
        mono: ["'JetBrains Mono'", "'Fira Code'", "monospace"],
        sans: ["'Inter'", "'Segoe UI'", "sans-serif"],
      },
      boxShadow: {
        glow: "0 0 15px rgba(0, 240, 255, 0.15)",
        "glow-lg": "0 0 25px rgba(112, 0, 255, 0.25)",
      },
      
      
      transitionDuration: {
        150: "150ms",
        180: "180ms",
        200: "200ms",
        220: "220ms",
      },
      keyframes: {
        toastIn: {
          "0%": { opacity: "0", transform: "translateY(6px) scale(0.98)" },
          "100%": { opacity: "1", transform: "translateY(0) scale(1)" },
        },
        modalIn: {
          "0%": { opacity: "0", transform: "translateY(8px) scale(0.97)" },
          "100%": { opacity: "1", transform: "translateY(0) scale(1)" },
        },
        overlayIn: {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
        keycapPress: {
          "0%": { transform: "scale(1)" },
          "40%": { transform: "scale(0.96)" },
          "100%": { transform: "scale(1)" },
        },
        rowIn: {
          "0%": { opacity: "0", transform: "translateY(-4px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
        rowOut: {
          "0%": { opacity: "1", transform: "scale(1)", maxHeight: "400px" },
          "100%": { opacity: "0", transform: "scale(0.98)", maxHeight: "0px" },
        },
        pulseSoft: {
          "0%, 100%": { opacity: "1" },
          "50%": { opacity: "0.55" },
        },
      },
      animation: {
        "toast-in": "toastIn 200ms ease-out",
        "modal-in": "modalIn 200ms cubic-bezier(0.16, 1, 0.3, 1)",
        "overlay-in": "overlayIn 180ms ease-out",
        "keycap-press": "keycapPress 160ms ease-out",
        "row-in": "rowIn 180ms ease-out",
        "row-out": "rowOut 180ms ease-in forwards",
        "pulse-soft": "pulseSoft 2s ease-in-out infinite",
      },
    },
  },
  plugins: [],
};
