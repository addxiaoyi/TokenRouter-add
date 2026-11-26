import autoprefixer from 'autoprefixer'
import tailwindcss from 'tailwindcss'

// postcss.config.js generates Tailwind utilities and prefixes browser styles.
// The supplied application font keeps its original weight and glyph coverage;
// no font-specific transformation runs before Tailwind resolves the shared tokens
// used by application styles and components.

export default {
  plugins: [tailwindcss(), autoprefixer()],
}
