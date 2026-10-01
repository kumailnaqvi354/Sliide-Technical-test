package articles

import (
	"errors"
	"time"
)

type Article struct {
	ID          string
	Title       string
	Summary     string
	ImagePath   string
	Author      string
	Source      string
	Category    string
	PublishedAt time.Time
	Disabled    bool

	// The most recent status change an editor asked for, or nil if there has
	// never been one. Compare with Disabled to tell whether it has landed.
	LatestRequest *StatusRequest
}

type ArticleDetails struct {
	Article Article
	Body    string
}

// Action is a requested status. The values match the queue message contract.
type Action string

const (
	ActionDisable Action = "disable"
	ActionEnable  Action = "enable"
)

func (a Action) disables() bool {
	return a == ActionDisable
}

// StatusRequest is a status change an editor asked for. It is recorded when
// the change is published, which is all we can know: the consumer applies it
// later, if at all.
type StatusRequest struct {
	// Also sent as the queue message's trace_id.
	ID          string
	ArticleID   string
	Action      Action
	RequestedAt time.Time
}

type ChangeState int

const (
	// There is no request, or the latest one has been applied.
	ChangeApplied ChangeState = iota
	// The latest request has not been applied yet, but may still be.
	ChangePending
	// The latest request has not been applied within the time the queue
	// would normally take, so it has probably been lost or overtaken.
	ChangeOverdue
)

// overdueAfter is how long a request may stay unapplied before we stop
// expecting it to land. The queue holds a message for 5s, then gives the
// consumer three attempts 30s apart before dead-lettering it, so a request
// that will ever succeed normally has by about 70s.
const overdueAfter = 2 * time.Minute

// changeState reports where the latest request stands against the status the
// consumer has actually applied.
func changeState(disabled bool, latest *StatusRequest, now time.Time) ChangeState {
	if latest == nil || latest.Action.disables() == disabled {
		return ChangeApplied
	}

	if now.Sub(latest.RequestedAt) < overdueAfter {
		return ChangePending
	}

	return ChangeOverdue
}

var (
	ErrAlreadyInState   = errors.New("article is already in the requested state")
	ErrChangeInProgress = errors.New("another status change is still being applied")
)

// planStatusChange decides what a new request for action should do. It returns
// the pending request to reuse when the same change is already in flight, nil
// when a new request should be published, or an error when it should be
// refused.
//
// Refusing the opposite change while one is pending is what keeps the
// unordered queue safe: with only one change in flight per article there is
// nothing for it to reorder.
func planStatusChange(disabled bool, latest *StatusRequest, action Action, now time.Time) (*StatusRequest, error) {
	switch changeState(disabled, latest, now) {
	case ChangePending:
		if latest.Action == action {
			return latest, nil
		}

		return nil, ErrChangeInProgress
	case ChangeApplied:
		if action.disables() == disabled {
			return nil, ErrAlreadyInState
		}
	case ChangeOverdue:
		// Anything goes: the editor may retry, or ask for the opposite to
		// cancel a change that is stuck.
	}

	return nil, nil
}
