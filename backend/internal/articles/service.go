package articles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	api "github.com/sliide/articles-backend/pkg/articles/api"
)

// Publisher puts a message on the article status change queue.
type Publisher interface {
	Publish(ctx context.Context, body string) error
}

// publishTimeout bounds how long an editor waits on the queue. The article row
// stays locked while we wait, so this must stay short.
const publishTimeout = 5 * time.Second

// Any 8-4-4-4-12 hex value, the same check the consumer applies. Postgres
// accepts ids that a strict RFC 9562 check would reject.
var articleIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Service struct {
	api.UnimplementedArticleAPIServer

	repository *Repository
	publisher  Publisher
	logger     *slog.Logger
	now        func() time.Time
}

func NewService(repository *Repository, publisher Publisher, logger *slog.Logger) *Service {
	return &Service{repository: repository, publisher: publisher, logger: logger, now: time.Now}
}

func (s *Service) GetArticles(ctx context.Context, _ *api.GetArticlesRequest) (*api.GetArticlesResponse, error) {
	found, err := s.repository.List()
	if err != nil {
		s.logger.ErrorContext(ctx, "could not list articles", "error", err)

		return nil, err
	}

	now := s.now()
	response := &api.GetArticlesResponse{Articles: make([]*api.Article, 0, len(found))}
	for _, article := range found {
		response.Articles = append(response.Articles, toProtoArticle(article, now))
	}

	return response, nil
}

func (s *Service) GetArticleDetails(ctx context.Context, req *api.GetArticleDetailsRequest) (*api.GetArticleDetailsResponse, error) {
	details, err := s.repository.Get(req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "article not found")
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "could not get article", "article_id", req.GetId(), "error", err)

		return nil, err
	}

	return &api.GetArticleDetailsResponse{
		Article: &api.ArticleDetails{
			Article: toProtoArticle(details.Article, s.now()),
			Body:    proto.String(details.Body),
		},
	}, nil
}

func (s *Service) RequestArticleStatusChange(ctx context.Context, req *api.RequestArticleStatusChangeRequest) (*api.RequestArticleStatusChangeResponse, error) {
	if !articleIDPattern.MatchString(req.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "id must be a UUID")
	}

	action, ok := fromProtoAction(req.GetAction())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "action must be DISABLE or ENABLE")
	}

	now := s.now()
	request, created, err := s.repository.RequestStatusChange(ctx, req.GetId(), action, now, s.publish)

	var publishErr *publishError
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, status.Error(codes.NotFound, "article not found")
	case errors.Is(err, ErrAlreadyInState), errors.Is(err, ErrChangeInProgress):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	case errors.As(err, &publishErr):
		s.logger.ErrorContext(ctx, "could not publish article status change",
			"article_id", req.GetId(), "action", action, "error", err)

		return nil, status.Error(codes.Unavailable, "could not send the change, please try again")
	case err != nil:
		s.logger.ErrorContext(ctx, "could not request article status change",
			"article_id", req.GetId(), "action", action, "error", err)

		return nil, status.Error(codes.Internal, "could not request the change")
	}

	s.logger.InfoContext(ctx, "article status change requested",
		"trace_id", request.ID, "article_id", request.ArticleID, "action", request.Action, "new", created)

	return &api.RequestArticleStatusChangeResponse{
		StatusChange: toProtoStatusChange(request, ChangePending),
	}, nil
}

// statusChangedEvent is the message the consumer expects. See
// external/README.md for the contract.
type statusChangedEvent struct {
	Type      string `json:"type"`
	ArticleID string `json:"article_id"`
	Action    Action `json:"action"`
	TraceID   string `json:"trace_id"`
}

const eventTypeStatusChanged = "article.status.changed"

type publishError struct{ err error }

func (e *publishError) Error() string { return fmt.Sprintf("publish: %v", e.err) }
func (e *publishError) Unwrap() error { return e.err }

func (s *Service) publish(ctx context.Context, request StatusRequest) error {
	body, err := json.Marshal(statusChangedEvent{
		Type:      eventTypeStatusChanged,
		ArticleID: request.ArticleID,
		Action:    request.Action,
		TraceID:   request.ID,
	})
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	if err := s.publisher.Publish(ctx, string(body)); err != nil {
		return &publishError{err: err}
	}

	return nil
}

func fromProtoAction(action api.ArticleStatusAction) (Action, bool) {
	switch action {
	case api.ArticleStatusAction_ARTICLE_STATUS_ACTION_DISABLE:
		return ActionDisable, true
	case api.ArticleStatusAction_ARTICLE_STATUS_ACTION_ENABLE:
		return ActionEnable, true
	default:
		return "", false
	}
}

func toProtoAction(action Action) api.ArticleStatusAction {
	if action.disables() {
		return api.ArticleStatusAction_ARTICLE_STATUS_ACTION_DISABLE
	}

	return api.ArticleStatusAction_ARTICLE_STATUS_ACTION_ENABLE
}

func toProtoStatusChange(request StatusRequest, state ChangeState) *api.StatusChange {
	protoState := api.StatusChangeState_STATUS_CHANGE_STATE_PENDING
	if state == ChangeOverdue {
		protoState = api.StatusChangeState_STATUS_CHANGE_STATE_OVERDUE
	}

	return &api.StatusChange{
		TraceId:     proto.String(request.ID),
		Action:      toProtoAction(request.Action).Enum(),
		RequestedAt: timestamppb.New(request.RequestedAt),
		State:       protoState.Enum(),
	}
}

func toProtoArticle(article Article, now time.Time) *api.Article {
	protoArticle := &api.Article{
		Id:          proto.String(article.ID),
		Title:       proto.String(article.Title),
		Summary:     proto.String(article.Summary),
		ImagePath:   proto.String(article.ImagePath),
		Author:      proto.String(article.Author),
		Source:      proto.String(article.Source),
		Category:    proto.String(article.Category),
		PublishedAt: timestamppb.New(article.PublishedAt),
		Disabled:    proto.Bool(article.Disabled),
	}

	if state := changeState(article.Disabled, article.LatestRequest, now); state != ChangeApplied {
		protoArticle.PendingChange = toProtoStatusChange(*article.LatestRequest, state)
	}

	return protoArticle
}
