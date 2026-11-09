package api

import (
	"net/http"

	"github.com/tfenng/tasks/task2go/internal/store"
)

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	p, apiErr := parsePage(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	from, apiErr := parseOptionalTime(r, "from")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	to, apiErr := parseOptionalTime(r, "to")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	items, total, err := s.Store.ListAudit(ctx, s.Store.Pool, p.Limit, p.Offset,
		parseOptionalString(r, "action"), parseOptionalString(r, "resource_type"), from, to)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	p.setTotal(total)
	actorIDs := make([]string, 0)
	seen := map[string]bool{}
	for _, item := range items {
		if item.ActorUserID != nil && !seen[*item.ActorUserID] {
			seen[*item.ActorUserID] = true
			actorIDs = append(actorIDs, *item.ActorUserID)
		}
	}
	usernames, _ := s.Store.UsernamesByIDs(ctx, s.Store.Pool, actorIDs)
	out := make([]AuditEventOut, 0, len(items))
	for _, item := range items {
		out = append(out, auditOut(item, usernames))
	}
	writeJSON(w, 200, AuditEventList{Items: out, Page: p.Page, PageSize: p.PageSize, Total: p.Total, TotalPages: p.TotalPages})
}

func auditOut(item store.AuditEvent, usernames map[string]string) AuditEventOut {
	var username *string
	if item.ActorUserID != nil {
		if name, ok := usernames[*item.ActorUserID]; ok {
			username = &name
		}
	}
	return AuditEventOut{
		ID: item.ID, ActorUserID: item.ActorUserID, ActorUsername: username,
		Action: item.Action, ResourceType: item.ResourceType, ResourceID: item.ResourceID,
		Outcome: item.Outcome, IPAddress: item.IPAddress, UserAgent: item.UserAgent,
		CorrelationID: item.CorrelationID, Details: item.Details.Map(), CreatedAt: item.CreatedAt,
	}
}
