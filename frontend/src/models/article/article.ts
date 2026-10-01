export interface Article {
  id: string;
  title: string;
  summary: string;

  /** Relative to the image CDN base URL, e.g. `/articles/technology-01.svg`. */
  imagePath: string;

  author: string;
  source: string;
  category: string;
  publishedAt: Date;

  /**
   * Whether the article is hidden from users. Applied asynchronously, so this
   * is the last state written rather than necessarily the most recently
   * requested one.
   */
  disabled: boolean;

  /**
   * The editor's latest status change, while it has been requested but not
   * yet applied. Null when there is none, or it has landed.
   */
  pendingChange: PendingStatusChange | null;
}

export type StatusAction = 'disable' | 'enable';

export interface PendingStatusChange {
  /** Also the queue message's trace id, for following it through the logs. */
  traceId: string;
  action: StatusAction;
  requestedAt: Date;

  /**
   * `pending` while it may still land. `overdue` once it has taken longer than
   * the queue ever should, so it has probably been lost, and may be retried.
   */
  state: 'pending' | 'overdue';
}

export interface ArticleDetails extends Article {
  body: string;
}
