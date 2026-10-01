import { timestampDate } from '@bufbuild/protobuf/wkt';
import { TRPCError } from '@trpc/server';

import * as grpc from '@server/generated/grpc/article_pb';
import type {
  Article,
  ArticleDetails,
  PendingStatusChange,
  StatusAction,
} from '@models/article/article';

export function mapGrpcArticle(article: grpc.Article): Article {
  return {
    id: article.id ?? '',
    title: article.title ?? '',
    summary: article.summary ?? '',
    imagePath: article.imagePath ?? '',
    author: article.author ?? '',
    source: article.source ?? '',
    category: article.category ?? '',
    publishedAt: article.publishedAt ? timestampDate(article.publishedAt) : new Date(0),
    disabled: article.disabled ?? false,
    pendingChange: article.pendingChange ? mapGrpcStatusChange(article.pendingChange) : null,
  };
}

export function mapGrpcArticleDetails(details: grpc.ArticleDetails): ArticleDetails {
  if (!details.article) {
    throw new TRPCError({
      code: 'INTERNAL_SERVER_ERROR',
      message: 'Article details are missing the article.',
    });
  }

  return {
    ...mapGrpcArticle(details.article),
    body: details.body ?? '',
  };
}

export function mapGrpcStatusChange(change: grpc.StatusChange): PendingStatusChange {
  const action = fromGrpcAction(change.action);
  if (!action) {
    throw new TRPCError({
      code: 'INTERNAL_SERVER_ERROR',
      message: 'The API returned a status change with no action.',
    });
  }

  return {
    traceId: change.traceId ?? '',
    action,
    requestedAt: change.requestedAt ? timestampDate(change.requestedAt) : new Date(0),
    // Anything but an explicit OVERDUE is treated as still in flight, so an
    // unknown state never unlocks the button early.
    state: change.state === grpc.StatusChangeState.OVERDUE ? 'overdue' : 'pending',
  };
}

export function toGrpcAction(action: StatusAction): grpc.ArticleStatusAction {
  return action === 'disable' ? grpc.ArticleStatusAction.DISABLE : grpc.ArticleStatusAction.ENABLE;
}

function fromGrpcAction(action: grpc.ArticleStatusAction | undefined): StatusAction | null {
  switch (action) {
    case grpc.ArticleStatusAction.DISABLE:
      return 'disable';
    case grpc.ArticleStatusAction.ENABLE:
      return 'enable';
    default:
      return null;
  }
}
