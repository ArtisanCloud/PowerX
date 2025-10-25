import type { Config } from 'tailwindcss'

export default <Config>{
  darkMode: 'class',
  content: [
    "./components/**/*.{js,vue,ts}",
    "./layouts/**/*.vue", 
    "./pages/**/*.vue",
    "./plugins/**/*.{js,ts}",
    "./app.vue",
    "./error.vue",
    "./app/**/*.{js,vue,ts}"
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}