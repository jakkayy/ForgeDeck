package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/auth"
)

const (
	maxBodyForFields = 64 << 10
	maxResponseCopy  = 64 << 10
)

// Middleware records the successful requests that Rules describes
func (rec *Recorder) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rule, ok := Rules[r.Method+" "+chi.RouteContext(r.Context()).RoutePattern()]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		// resolve what the request is about before the handler may delete it
		target := rec.resolve(r.Context(), rule, r)
		metadata := rec.requestFields(r, rule)

		capture := &captureWriter{ResponseWriter: w, limit: maxResponseCopy}
		next.ServeHTTP(capture, r)
		if capture.status() >= 300 {
			return
		}

		event := Event{
			ProjectID: target.projectID, Action: rule.Action, TargetType: rule.TargetType,
			TargetID: target.id, TargetName: target.name, Metadata: metadata,
		}
		event.Actor = rule.Actor
		if user, ok := auth.MaybeUserFrom(r.Context()); ok {
			event.ActorUserID = &user.ID
		}
		if rule.FromResponse {
			fromResponse(&event, capture.body.Bytes(), rule)
		}
		if rule.ResponseServer {
			rec.serverFromResponse(r.Context(), &event, capture.body.Bytes())
		}
		if capture.status() == http.StatusCreated {
			event.Metadata = withMeta(event.Metadata, "created", true)
		}
		rec.Record(context.WithoutCancel(r.Context()), event)
	})
}

type resolved struct {
	projectID *uuid.UUID
	id, name  string
}

func (rec *Recorder) resolve(ctx context.Context, rule Rule, r *http.Request) resolved {
	out := resolved{}
	if rule.TargetIDParam != "" {
		out.id = chi.URLParam(r, rule.TargetIDParam)
	}
	if rule.TargetNameParam != "" {
		out.name = chi.URLParam(r, rule.TargetNameParam)
	}
	if rule.Kind == KindNone {
		return out
	}
	id, err := uuid.Parse(chi.URLParam(r, rule.Param))
	if err != nil {
		return out
	}
	projectID, name, err := rec.lookup(ctx, rule.Kind, id)
	if err != nil {
		return out // the handler will reject it; there is nothing to record
	}
	out.projectID = &projectID
	// the looked-up resource is the event's target only when the rule is about that kind of resource
	if kindType[rule.Kind] == rule.TargetType {
		if out.id == "" {
			out.id = id.String()
		}
		if out.name == "" {
			out.name = name
		}
	}
	return out
}

// kindType is the target type of a resource of each Kind
var kindType = map[Kind]string{
	KindProject: "project", KindApplication: "application", KindEnvironment: "environment",
	KindRelease: "release", KindJob: "job", KindGitConnection: "git_connection",
	KindRegistry: "registry", KindServer: "server",
}

var errUnknownKind = errors.New("unknown audit kind")

// lookup finds the project and a display name of a resource
func (rec *Recorder) lookup(ctx context.Context, kind Kind, id uuid.UUID) (uuid.UUID, string, error) {
	q := rec.queries
	switch kind {
	case KindProject:
		p, err := q.GetProject(ctx, id)
		return p.ID, p.Name, err
	case KindApplication:
		a, err := q.GetApplication(ctx, id)
		return a.ProjectID, a.Name, err
	case KindEnvironment:
		return rec.environmentProject(ctx, id)
	case KindRelease:
		release, err := q.GetRelease(ctx, id)
		if err != nil {
			return uuid.Nil, "", err
		}
		projectID, name, err := rec.environmentProject(ctx, release.EnvironmentID)
		return projectID, name + " @ " + shortSHA(release.CommitSha), err
	case KindJob:
		j, err := q.GetJob(ctx, id)
		return j.ProjectID, j.Type, err
	case KindGitConnection:
		g, err := q.GetGitConnection(ctx, id)
		return g.ProjectID, g.DisplayName, err
	case KindRegistry:
		reg, err := q.GetRegistryConnection(ctx, id)
		return reg.ProjectID, reg.Name, err
	case KindServer:
		s, err := q.GetServer(ctx, id)
		return s.Server.ProjectID, s.Server.Name, err
	default:
		return uuid.Nil, "", errUnknownKind
	}
}

func (rec *Recorder) environmentProject(ctx context.Context, environmentID uuid.UUID) (uuid.UUID, string, error) {
	env, err := rec.queries.GetEnvironment(ctx, environmentID)
	if err != nil {
		return uuid.Nil, "", err
	}
	app, err := rec.queries.GetApplication(ctx, env.ApplicationID)
	return app.ProjectID, env.Name, err
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// requestFields copies the rule's whitelisted JSON body fields into metadata and restores the body
func (rec *Recorder) requestFields(r *http.Request, rule Rule) map[string]any {
	if len(rule.RequestFields) == 0 || r.Body == nil {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyForFields+1))
	if err != nil {
		return nil
	}
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(body), r.Body), r.Body}
	if len(body) > maxBodyForFields {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return nil
	}
	out := map[string]any{}
	for _, name := range rule.RequestFields {
		if raw, ok := fields[name]; ok {
			var value any
			if json.Unmarshal(raw, &value) == nil {
				out[name] = value
			}
		}
	}
	return out
}

func withMeta(metadata map[string]any, key string, value any) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata[key] = value
	return metadata
}

// fromResponse fills what a create route's response says
func fromResponse(e *Event, body []byte, rule Rule) {
	var doc struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		ServerName  string `json:"server_name"`
		DisplayName string `json:"display_name"`
		ProjectID   string `json:"project_id"`
		UserID      string `json:"user_id"`
		User        struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return
	}
	if e.TargetID == "" {
		e.TargetID = firstNonEmpty(doc.ID, doc.UserID, doc.User.ID)
	}
	if e.TargetName == "" {
		e.TargetName = firstNonEmpty(doc.Name, doc.Email, doc.ServerName, doc.DisplayName, doc.User.Email)
	}
	if e.ProjectID == nil {
		if rule.TargetType == "project" {
			if id, err := uuid.Parse(doc.ID); err == nil {
				e.ProjectID = &id
			}
		} else if id, err := uuid.Parse(doc.ProjectID); err == nil {
			e.ProjectID = &id
		}
	}
	if rule.Action == "user.login" && e.ActorUserID == nil {
		if id, err := uuid.Parse(doc.User.ID); err == nil {
			e.ActorUserID = &id
		}
	}
}

// serverFromResponse names the server an Agent enrolled as, and its project
func (rec *Recorder) serverFromResponse(ctx context.Context, e *Event, body []byte) {
	var doc struct {
		ServerID string `json:"server_id"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return
	}
	id, err := uuid.Parse(doc.ServerID)
	if err != nil {
		return
	}
	projectID, name, err := rec.lookup(ctx, KindServer, id)
	if err != nil {
		return
	}
	e.ProjectID, e.TargetID, e.TargetName = &projectID, id.String(), name
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// captureWriter remembers the status and the start of the response body
type captureWriter struct {
	http.ResponseWriter
	code  int
	body  bytes.Buffer
	limit int
}

func (c *captureWriter) WriteHeader(code int) {
	if c.code == 0 {
		c.code = code
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	if c.code == 0 {
		c.code = http.StatusOK
	}
	if room := c.limit - c.body.Len(); room > 0 {
		c.body.Write(p[:min(len(p), room)])
	}
	return c.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController reach the real writer (SSE and long-poll need it)
func (c *captureWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *captureWriter) status() int {
	if c.code == 0 {
		return http.StatusOK
	}
	return c.code
}
