package jobs

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/store"
)

type Filter struct {
	ProjectID     *uuid.UUID
	ApplicationID *uuid.UUID
	ServerID      *uuid.UUID
	Status        *Status
	Type          *string
}

type HistoryCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

func (c HistoryCursor) Encode() string {
	raw := strconv.FormatInt(c.CreatedAt.UnixMicro(), 10) + "_" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeHistoryCursor(s string) (*HistoryCursor, bool) {
	if s == "" {
		return nil, true
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, false
	}
	micros, id, found := strings.Cut(string(raw), "_")
	us, errT := strconv.ParseInt(micros, 10, 64)
	parsedID, errID := uuid.Parse(id)
	if !found || errT != nil || errID != nil {
		return nil, false
	}
	return &HistoryCursor{CreatedAt: time.UnixMicro(us), ID: parsedID}, true
}

// History lists jobs newest first across the user's projects, with the next cursor
func (s *Service) History(ctx context.Context, userID uuid.UUID, f Filter, after *HistoryCursor, limit int32) ([]Job, *HistoryCursor, error) {
	params := store.ListJobsForUserParams{
		UserID: userID, ProjectID: f.ProjectID, ApplicationID: f.ApplicationID,
		ServerID: f.ServerID, Type: f.Type, PageLimit: limit,
	}
	if f.Status != nil {
		status := string(*f.Status)
		params.Status = &status
	}
	if after != nil {
		params.BeforeCreatedAt, params.BeforeID = &after.CreatedAt, &after.ID
	}
	rows, err := s.queries.ListJobsForUser(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list jobs: %w", err)
	}
	jobs := make([]Job, len(rows))
	for i, row := range rows {
		jobs[i] = toJob(row)
	}
	if int32(len(rows)) < limit { //nolint:gosec // len is bounded by limit
		return jobs, nil, nil
	}
	last := rows[len(rows)-1]
	return jobs, &HistoryCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
}
