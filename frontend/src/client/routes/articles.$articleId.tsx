import { useSuspenseQuery } from '@tanstack/react-query';
import { createFileRoute, Link } from '@tanstack/react-router';

import { formatPublished } from '@client/lib/format';
import { imageUrl } from '@client/lib/image';
import { refetchWhilePending } from '@client/lib/status-change';
import { useTRPC } from '@client/trpc';
import { StatusBadge } from './-components/status-badge';
import { StatusControl } from './-components/status-control';

export const Route = createFileRoute('/articles/$articleId')({
  loader: async ({ context, params }) => {
    await context.queryClient.query(
      context.trpc.articles.getArticleDetails.queryOptions({ id: params.articleId }),
    );
  },
  component: ArticleDetailsPage,
  pendingComponent: () => <p className="text-slate-500">Loading article…</p>,
});

function ArticleDetailsPage() {
  const { articleId } = Route.useParams();
  const trpc = useTRPC();
  const { data: article } = useSuspenseQuery({
    ...trpc.articles.getArticleDetails.queryOptions({ id: articleId }),
    refetchInterval: (query) => refetchWhilePending(query.state.data && [query.state.data]),
  });

  return (
    <article>
      <Link to="/" className="text-sm text-slate-500 hover:text-slate-900">
        ← All articles
      </Link>

      <div className="mt-4 flex items-start gap-3">
        <h1 className="flex-1 text-3xl font-semibold">{article.title}</h1>
        <StatusBadge disabled={article.disabled} pendingChange={article.pendingChange} />
      </div>

      <p className="mt-2 text-sm text-slate-500">
        {article.author} · {article.source} · <span className="capitalize">{article.category}</span>{' '}
        · {formatPublished(article.publishedAt)}
      </p>

      <StatusControl article={article} />

      <img
        src={imageUrl(article.imagePath)}
        alt=""
        className="mt-6 h-72 w-full rounded-lg object-cover"
      />

      <p className="mt-6 text-lg text-slate-700">{article.summary}</p>

      <div className="mt-4 space-y-4 text-slate-800">
        {article.body.split('\n\n').map((paragraph) => (
          <p key={paragraph.slice(0, 32)}>{paragraph}</p>
        ))}
      </div>
    </article>
  );
}
