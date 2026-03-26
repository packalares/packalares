// ═══════════════════════════════════════════════════════════════════
// Packalares — Shared utility helpers
// De-duplicated from Vue pages into a single source of truth
// ═══════════════════════════════════════════════════════════════════

/**
 * Format byte count into human-readable string (e.g. "1.2 GB")
 */
export function formatBytes(b?: number): string {
  if (b == null || b === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(b) / Math.log(1024));
  return (b / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
}

/**
 * Format a byte-per-second rate into human-readable string (e.g. "1.2 MB/s")
 */
export function formatRate(bps?: number): string {
  if (bps == null || bps === 0) return '0 B/s';
  if (bps < 1024) return bps.toFixed(0) + ' B/s';
  if (bps < 1024 * 1024) return (bps / 1024).toFixed(1) + ' KB/s';
  return (bps / 1024 / 1024).toFixed(1) + ' MB/s';
}

/**
 * Format uptime in seconds to a human-readable duration (e.g. "3d 2h" or "45m")
 */
export function formatUptime(s?: number): string {
  if (s == null || s === 0) return '--';
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

/**
 * Return a Quasar text-color class based on usage percentage
 * Red for >= 80%, amber for >= 50%, green otherwise
 */
export function usageColor(pct: number): string {
  return pct >= 80 ? 'text-red-5' : pct >= 50 ? 'text-amber-7' : 'text-green-5';
}

/**
 * Return a Quasar component color string based on usage percentage
 */
export function usageQColor(pct: number): string {
  return pct >= 80 ? 'red-6' : pct >= 50 ? 'amber-7' : 'green-6';
}

/**
 * Format a load average array to a display string
 */
export function fmtLoad(l: number[]): string {
  return l.map(v => v.toFixed(2)).join('  /  ');
}
