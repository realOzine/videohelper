export function tabIndexForKey(key: string, current: number, count: number): number | null {
  if (count < 1 || current < 0 || current >= count) return null;
  switch (key) {
    case 'ArrowRight': return (current + 1) % count;
    case 'ArrowLeft': return (current + count - 1) % count;
    case 'Home': return 0;
    case 'End': return count - 1;
    default: return null;
  }
}
