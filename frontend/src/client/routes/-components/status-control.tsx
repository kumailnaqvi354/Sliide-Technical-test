import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import type { ArticleDetails, StatusAction } from '@models/article/article';
import { formatAgo } from '@client/lib/format';
import { describeAction, useNow } from '@client/lib/status-change';
import { useTRPC } from '@client/trpc';

/**
 * Lets an editor disable or enable an article. The change is applied
 * asynchronously by another service, so this only ever reports what the server
 * knows: requested, applied, or requested but overdue.
 */
export function StatusControl({ article }: { article: ArticleDetails }) {
  const trpc = useTRPC();
  const queryClient = useQueryClient();
  const pending = article.pendingChange;
  const now = useNow(pending?.state === 'pending');

  // The change this editor asked for in this visit, so we can confirm it when
  // it lands. Everything else comes from the server.
  const [requested, setRequested] = useState<{ traceId: string; action: StatusAction } | null>(
    null,
  );

  const detailsKey = trpc.articles.getArticleDetails.queryKey({ id: article.id });

  const mutation = useMutation(
    trpc.articles.requestStatusChange.mutationOptions({
      onSuccess: (change) => {
        setRequested({ traceId: change.traceId, action: change.action });

        // Show the accepted request straight away rather than after the next
        // read. This is server state, not an optimistic guess: the applied
        // status is left as it is.
        queryClient.setQueryData(detailsKey, (current) =>
          current ? { ...current, pendingChange: change } : current,
        );
      },
      onSettled: () =>
        Promise.all([
          queryClient.invalidateQueries({ queryKey: detailsKey }),
          queryClient.invalidateQueries({ queryKey: trpc.articles.getArticles.queryKey() }),
        ]),
    }),
  );

  const request = (action: StatusAction) => {
    setRequested(null);
    mutation.mutate({ id: article.id, action });
  };

  const confirmed =
    requested !== null &&
    pending === null &&
    article.disabled === (requested.action === 'disable');

  // Retrying an overdue change repeats it. Otherwise offer the opposite of
  // what is applied.
  const nextAction: StatusAction =
    pending?.state === 'overdue' ? pending.action : article.disabled ? 'enable' : 'disable';

  const locked = mutation.isPending || pending?.state === 'pending';

  return (
    <section
      aria-live="polite"
      className="mt-6 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-slate-200 bg-white p-4"
    >
      <div className="min-w-0 flex-1 text-sm">
        {pending?.state === 'pending' && (
          <p className="text-amber-800">
            <strong className="font-medium">{describeAction(pending)} requested</strong>{' '}
            {formatAgo(pending.requestedAt, now)}. It usually takes a few seconds to apply, and this
            page will update when it has.
          </p>
        )}

        {pending?.state === 'overdue' && (
          <p className="text-orange-800">
            <strong className="font-medium">{describeAction(pending)} was requested</strong>{' '}
            {formatAgo(pending.requestedAt, now)} but has not been applied. It may have failed. You
            can try again.
          </p>
        )}

        {!pending && confirmed && (
          <p className="text-emerald-700">
            <strong className="font-medium">
              Article {requested.action === 'disable' ? 'disabled' : 'enabled'}.
            </strong>{' '}
            {requested.action === 'disable'
              ? 'It is no longer shown to readers.'
              : 'It is visible to readers again.'}
          </p>
        )}

        {!pending && !confirmed && (
          <p className="text-slate-600">
            {article.disabled
              ? 'This article is disabled and hidden from readers.'
              : 'This article is live and visible to readers.'}
          </p>
        )}

        {mutation.isError && (
          <p role="alert" className="mt-1 text-red-700">
            Could not request the change: {mutation.error.message}
          </p>
        )}
      </div>

      <button
        type="button"
        disabled={locked}
        onClick={() => request(nextAction)}
        className={
          nextAction === 'disable'
            ? 'rounded-md border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50'
            : 'rounded-md border border-emerald-300 bg-white px-3 py-1.5 text-sm font-medium text-emerald-700 hover:bg-emerald-50 disabled:cursor-not-allowed disabled:opacity-50'
        }
      >
        {buttonLabel(nextAction, mutation.isPending, pending?.state)}
      </button>
    </section>
  );
}

function buttonLabel(
  action: StatusAction,
  sending: boolean,
  state: 'pending' | 'overdue' | undefined,
): string {
  if (sending) {
    return 'Sending…';
  }
  if (state === 'pending') {
    return action === 'disable' ? 'Disabling…' : 'Enabling…';
  }
  if (state === 'overdue') {
    return `Try ${action} again`;
  }

  return action === 'disable' ? 'Disable article' : 'Enable article';
}
