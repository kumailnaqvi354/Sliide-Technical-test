package articles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("article not found")

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// latestRequestJoin attaches each article's most recent status request, if it
// has one, as r.id, r.action and r.requested_at.
const latestRequestJoin = `
	LEFT JOIN LATERAL (
		SELECT id, action, requested_at
		FROM article_status_requests
		WHERE article_id = a.id
		ORDER BY requested_at DESC, id DESC
		LIMIT 1
	) r ON TRUE`

const listQuery = `
	SELECT a.id, a.title, a.summary, a.image_path, a.author, a.source, a.category, a.published_at, a.disabled,
		r.id, r.action, r.requested_at
	FROM articles a` + latestRequestJoin + `
	ORDER BY a.published_at DESC, a.id`

// List returns every article, newest first.
func (r *Repository) List() (articles []Article, err error) {
	rows, err := r.db.Query(listQuery)
	if err != nil {
		return nil, fmt.Errorf("query articles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var article Article
		var latest nullableRequest
		if err := rows.Scan(
			&article.ID,
			&article.Title,
			&article.Summary,
			&article.ImagePath,
			&article.Author,
			&article.Source,
			&article.Category,
			&article.PublishedAt,
			&article.Disabled,
			&latest.id,
			&latest.action,
			&latest.requestedAt,
		); err != nil {
			return nil, fmt.Errorf("scan article: %w", err)
		}

		article.LatestRequest = latest.toRequest(article.ID)
		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate articles: %w", err)
	}

	return articles, nil
}

const getQuery = `
	SELECT a.id, a.title, a.summary, a.image_path, a.author, a.source, a.category, a.published_at, a.disabled, a.body,
		r.id, r.action, r.requested_at
	FROM articles a` + latestRequestJoin + `
	WHERE a.id = $1`

func (r *Repository) Get(id string) (ArticleDetails, error) {
	var details ArticleDetails
	var latest nullableRequest

	err := r.db.QueryRow(getQuery, id).Scan(
		&details.Article.ID,
		&details.Article.Title,
		&details.Article.Summary,
		&details.Article.ImagePath,
		&details.Article.Author,
		&details.Article.Source,
		&details.Article.Category,
		&details.Article.PublishedAt,
		&details.Article.Disabled,
		&details.Body,
		&latest.id,
		&latest.action,
		&latest.requestedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ArticleDetails{}, ErrNotFound
	}
	if err != nil {
		return ArticleDetails{}, fmt.Errorf("query article: %w", err)
	}

	details.Article.LatestRequest = latest.toRequest(details.Article.ID)

	return details, nil
}

const (
	lockArticleQuery = `SELECT disabled FROM articles WHERE id = $1 FOR UPDATE`

	latestRequestQuery = `
		SELECT id, action, requested_at
		FROM article_status_requests
		WHERE article_id = $1
		ORDER BY requested_at DESC, id DESC
		LIMIT 1`

	insertRequestQuery = `
		INSERT INTO article_status_requests (article_id, action, requested_at)
		VALUES ($1, $2, $3)
		RETURNING id`
)

// RequestStatusChange records a request for action against the article and
// hands it to publish, or returns the matching request already in flight. It
// reports whether a new request was created.
//
// The article row is locked for the duration, so concurrent requests for the
// same article are decided one at a time. The request is only committed if
// publish succeeds, so a failed publish never leaves a phantom pending change.
func (r *Repository) RequestStatusChange(
	ctx context.Context,
	articleID string,
	action Action,
	now time.Time,
	publish func(context.Context, StatusRequest) error,
) (StatusRequest, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return StatusRequest{}, false, fmt.Errorf("begin transaction: %w", err)
	}
	// A no-op once the transaction has been committed.
	defer tx.Rollback()

	var disabled bool
	err = tx.QueryRowContext(ctx, lockArticleQuery, articleID).Scan(&disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusRequest{}, false, ErrNotFound
	}
	if err != nil {
		return StatusRequest{}, false, fmt.Errorf("lock article: %w", err)
	}

	var latest nullableRequest
	err = tx.QueryRowContext(ctx, latestRequestQuery, articleID).Scan(&latest.id, &latest.action, &latest.requestedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return StatusRequest{}, false, fmt.Errorf("query latest request: %w", err)
	}

	existing, err := planStatusChange(disabled, latest.toRequest(articleID), action, now)
	if err != nil {
		return StatusRequest{}, false, err
	}
	if existing != nil {
		return *existing, false, nil
	}

	request := StatusRequest{ArticleID: articleID, Action: action, RequestedAt: now}
	if err := tx.QueryRowContext(ctx, insertRequestQuery, articleID, action, now).Scan(&request.ID); err != nil {
		return StatusRequest{}, false, fmt.Errorf("insert request: %w", err)
	}

	if err := publish(ctx, request); err != nil {
		return StatusRequest{}, false, err
	}

	// If this fails the message is already on the queue, so the change will
	// still apply, just without a record of the request. A transactional
	// outbox would close that gap.
	if err := tx.Commit(); err != nil {
		return StatusRequest{}, false, fmt.Errorf("commit request: %w", err)
	}

	return request, true, nil
}

// nullableRequest scans the columns of a status request that may not exist.
type nullableRequest struct {
	id          sql.NullString
	action      sql.NullString
	requestedAt sql.NullTime
}

func (n nullableRequest) toRequest(articleID string) *StatusRequest {
	if !n.id.Valid {
		return nil
	}

	return &StatusRequest{
		ID:          n.id.String,
		ArticleID:   articleID,
		Action:      Action(n.action.String),
		RequestedAt: n.requestedAt.Time,
	}
}
