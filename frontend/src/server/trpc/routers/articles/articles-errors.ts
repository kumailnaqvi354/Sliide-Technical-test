import { Code, ConnectError } from '@connectrpc/connect';
import { TRPCError } from '@trpc/server';

const codeMap: Partial<Record<Code, TRPCError['code']>> = {
  [Code.InvalidArgument]: 'BAD_REQUEST',
  [Code.NotFound]: 'NOT_FOUND',
  [Code.AlreadyExists]: 'CONFLICT',
  [Code.FailedPrecondition]: 'PRECONDITION_FAILED',
  [Code.PermissionDenied]: 'FORBIDDEN',
  [Code.Unauthenticated]: 'UNAUTHORIZED',
  [Code.DeadlineExceeded]: 'TIMEOUT',
  [Code.Unavailable]: 'INTERNAL_SERVER_ERROR',
};

/**
 * Maps a gRPC failure onto the equivalent tRPC error, so the client can tell a
 * bad request from a missing article from an outage. Anything unrecognised
 * stays an internal error rather than being reported as the caller's fault.
 */
export function toArticlesTRPCError(error: unknown): TRPCError {
  if (!(error instanceof ConnectError)) {
    return new TRPCError({
      code: 'INTERNAL_SERVER_ERROR',
      message: 'Unexpected error talking to the API.',
      cause: error,
    });
  }

  return new TRPCError({
    code: codeMap[error.code] ?? 'INTERNAL_SERVER_ERROR',
    message: error.rawMessage,
    cause: error,
  });
}
