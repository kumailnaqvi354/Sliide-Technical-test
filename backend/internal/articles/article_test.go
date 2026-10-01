package articles

import (
	"errors"
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func requestedAgo(action Action, age time.Duration) *StatusRequest {
	return &StatusRequest{ID: "trace", Action: action, RequestedAt: testNow.Add(-age)}
}

func TestChangeState(t *testing.T) {
	tests := []struct {
		name     string
		disabled bool
		latest   *StatusRequest
		want     ChangeState
	}{
		{"no request", false, nil, ChangeApplied},
		{"disable applied", true, requestedAgo(ActionDisable, time.Second), ChangeApplied},
		{"enable applied", false, requestedAgo(ActionEnable, time.Second), ChangeApplied},
		{"disable in flight", false, requestedAgo(ActionDisable, time.Second), ChangePending},
		{"enable in flight", true, requestedAgo(ActionEnable, time.Second), ChangePending},
		{"just under the threshold", false, requestedAgo(ActionDisable, overdueAfter-time.Nanosecond), ChangePending},
		{"at the threshold", false, requestedAgo(ActionDisable, overdueAfter), ChangeOverdue},
		{"long overdue", true, requestedAgo(ActionEnable, time.Hour), ChangeOverdue},
		{"old but applied", true, requestedAgo(ActionDisable, time.Hour), ChangeApplied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changeState(tt.disabled, tt.latest, testNow); got != tt.want {
				t.Errorf("changeState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanStatusChange(t *testing.T) {
	pendingDisable := requestedAgo(ActionDisable, time.Second)
	overdueDisable := requestedAgo(ActionDisable, time.Hour)

	tests := []struct {
		name         string
		disabled     bool
		latest       *StatusRequest
		action       Action
		wantExisting *StatusRequest
		wantErr      error
	}{
		{"first request", false, nil, ActionDisable, nil, nil},
		{"after an applied request", true, requestedAgo(ActionDisable, time.Hour), ActionEnable, nil, nil},
		{"already in that state", false, nil, ActionEnable, nil, ErrAlreadyInState},
		{"already applied", true, requestedAgo(ActionDisable, time.Second), ActionDisable, nil, ErrAlreadyInState},
		{"same change in flight is reused", false, pendingDisable, ActionDisable, pendingDisable, nil},
		{"opposite change in flight is refused", false, pendingDisable, ActionEnable, nil, ErrChangeInProgress},
		{"overdue may be retried", false, overdueDisable, ActionDisable, nil, nil},
		{"overdue may be cancelled", false, overdueDisable, ActionEnable, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing, err := planStatusChange(tt.disabled, tt.latest, tt.action, testNow)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if existing != tt.wantExisting {
				t.Errorf("existing = %v, want %v", existing, tt.wantExisting)
			}
		})
	}
}
