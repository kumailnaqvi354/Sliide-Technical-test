import type { PendingStatusChange } from '@models/article/article';
import { describeAction } from '@client/lib/status-change';

/**
 * The status that has actually been applied, plus a chip for any change that
 * has been requested but has not landed yet. The applied status is never
 * changed optimistically: a takedown must not look done before it is.
 */
export function StatusBadge({
  disabled,
  pendingChange,
}: {
  disabled: boolean;
  pendingChange: PendingStatusChange | null;
}) {
  return (
    <span className="flex shrink-0 items-center gap-1.5">
      {pendingChange && <PendingChip change={pendingChange} />}
      {disabled ? (
        <span className="rounded-full bg-red-100 px-2.5 py-0.5 text-xs font-medium text-red-700">
          Disabled
        </span>
      ) : (
        <span className="rounded-full bg-emerald-100 px-2.5 py-0.5 text-xs font-medium text-emerald-700">
          Live
        </span>
      )}
    </span>
  );
}

function PendingChip({ change }: { change: PendingStatusChange }) {
  if (change.state === 'overdue') {
    return (
      <span className="rounded-full bg-orange-100 px-2.5 py-0.5 text-xs font-medium text-orange-800">
        {describeAction(change)} not applied
      </span>
    );
  }

  return (
    <span className="flex items-center gap-1.5 rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-medium text-amber-800">
      <span className="size-1.5 animate-pulse rounded-full bg-amber-500" aria-hidden />
      {describeAction(change)} requested
    </span>
  );
}
