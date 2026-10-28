// API codes choose translated guidance; raw messages stay available in technical details.
export function errorSummaryKey(code?: string): string {
  switch (code) {
    case 'network': return 'err.network';
    case 'invalid': case 'invalid_input': case 'invalid_argument': return 'err.input';
    case 'not_ready': case 'media_not_ready': return 'err.notReady';
    case 'unavailable': return 'err.unavailable';
    case 'not_found': return 'err.missing';
    case 'conflict': return 'err.changed';
    case 'cancelled': case 'canceled': return 'desktop.cancelled';
    default: return 'err.generic';
  }
}

export const libraryEmptyKey = (project: string, status: string) => status ? 'l.emptyFilter' : project ? 'l.emptyProject' : 'l.empty';

export function runtimeMessageKey(stage?: string): string {
  switch (stage) {
    case 'extracting': case 'tools': case 'checking': return 'desktop.checking';
    case 'checking_models': return 'desktop.checkingModels';
    case 'python': case 'packages': return 'desktop.installing';
    case 'detecting': return 'desktop.detecting';
    case 'models': return 'desktop.models';
    case 'installed': case 'loading': return 'desktop.loading';
    default: return 'desktop.wait';
  }
}
