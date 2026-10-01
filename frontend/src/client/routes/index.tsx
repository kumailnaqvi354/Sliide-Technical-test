import { useSuspenseQuery } from '@tanstack/react-query';
import { createFileRoute, Link } from '@tanstack/react-router';

import { formatPublished } from '@client/lib/format';
import { imageUrl } from '@client/lib/image';
import { refetchWhilePending } from '@client/lib/status-change';
import { useTRPC } from '@client/trpc';
import { StatusBadge } from './-components/status-badge';

export const Route = createFileRoute('/')({
  loader: async ({ context }) => {
    await context.queryClient.query(context.trpc.articles.getArticles.queryOptions());
  },
  component: ArticleList,
  pendingComponent: () => <p className="text-slate-500">Loading articles…</p>,
});

function ArticleList() {
  const trpc = useTRPC();
  const { data: articles } = useSuspenseQuery({
    ...trpc.articles.getArticles.queryOptions(),
    refetchInterval: (query) => refetchWhilePending(query.state.data),
  });

  return (
    <>
      <div className="mb-6 flex items-baseline justify-between">
        <h1 className="text-2xl font-semibold">All articles</h1>
        <p className="text-sm text-slate-500">{articles.length} articles</p>
      </div>

      <ul className="divide-y divide-slate-200 overflow-hidden rounded-lg border border-slate-200 bg-white">
        {articles.map((article) => (
          <li key={article.id}>
            <Link
              to="/articles/$articleId"
              params={{ articleId: article.id }}
              className="flex gap-4 p-4 hover:bg-slate-50"
            >
              <img
                src={imageUrl(article.imagePath)}
                alt=""
                className="h-16 w-28 shrink-0 rounded object-cover"
              />

              <div className="min-w-0 flex-1">
                <div className="flex items-start gap-3">
                  <h2 className="flex-1 font-medium">{article.title}</h2>
                  <StatusBadge disabled={article.disabled} pendingChange={article.pendingChange} />
                </div>
                <p className="mt-1 line-clamp-2 text-sm text-slate-600">{article.summary}</p>
                <p className="mt-1.5 text-xs text-slate-500">
                  <span className="capitalize">{article.category}</span> · {article.source} ·{' '}
                  {formatPublished(article.publishedAt)}
                </p>
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </>
  );
}
