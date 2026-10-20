import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [tailwindcss()],
  server: {
    proxy: {
      "/device-presets": "http://127.0.0.1:8080",
      "/transform": "http://127.0.0.1:8080",
      "/inspect-identity": "http://127.0.0.1:8080",
      "/activity-details": "http://127.0.0.1:8080",
    },
  },
});
