// =====================================================================
// Packalares -- Theme composable
// Single source of truth for theme switching (dark/light)
// Uses data-theme attribute on <html> for CSS variable switching
// =====================================================================

import { ref } from 'vue';

const currentTheme = ref(localStorage.getItem('packalares_theme') || 'dark');

/**
 * Apply the given theme by setting [data-theme] on the document root.
 * CSS variables in variables.scss handle the rest via [data-theme="light"].
 */
export function applyTheme(theme: string): void {
  currentTheme.value = theme;
  const root = document.documentElement;

  if (theme === 'light') {
    root.setAttribute('data-theme', 'light');
  } else {
    root.removeAttribute('data-theme');
  }
}

/**
 * Composable for theme management.
 * Returns the reactive theme ref and the apply function.
 */
export function useTheme() {
  return {
    currentTheme,
    applyTheme,
  };
}
