import { useEffect, useState } from 'react';

import type { Article, PendingStatusChange } from '@models/article/article';

// Nothing tells us when the consumer applies a change, so while one is in
// flight we re-read the article to find out. Only while one is in flight, and
// slower once it is overdue: a late change can still land, but is unlikely to.
const PENDING_REFETCH_MS = 2_000;
const OVERDUE_REFETCH_MS = 15_000;

/** A react-query refetchInterval for whatever changes are shown on screen. */
export function refetchWhilePending(
  articles: Pick<Article, 'pendingChange'>[] | undefined,
): number | false {
  const states = (articles ?? []).map((article) => article.pendingChange?.state);

  if (states.includes('pending')) {
    return PENDING_REFETCH_MS;
  }
  if (states.includes('overdue')) {
    return OVERDUE_REFETCH_MS;
  }

  return false;
}

/** The current time, ticking every second while `active`. */
export function useNow(active: boolean): Date {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    if (!active) {
      return;
    }

    setNow(new Date());
    const timer = setInterval(() => setNow(new Date()), 1_000);

    return () => clearInterval(timer);
  }, [active]);

  return now;
}

export function describeAction(change: Pick<PendingStatusChange, 'action'>): string {
  return change.action === 'disable' ? 'Disable' : 'Enable';
}
