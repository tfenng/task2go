package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/core"
	"github.com/tfenng/tasks/task2go/internal/store"
)

func (t *TargetUpdate) UnmarshalJSON(data []byte) error {
	type alias TargetUpdate
	var aux struct {
		*alias
		CredentialID json.RawMessage `json:"credential_id"`
	}
	aux.alias = (*alias)(t)
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.CredentialID != nil {
		t.CredentialIDSet = true
		if string(aux.CredentialID) == "null" {
			t.CredentialID = nil
		} else if err := json.Unmarshal(aux.CredentialID, &t.CredentialID); err != nil {
			return err
		}
	}
	return nil
}

func validateTarget(db *Server, ctx context.Context, targetType string, config map[string]any, credentialID *string) *apiError {
	if config == nil {
		return validationErr("target config must be an object")
	}
	kindFor := map[string]map[string]bool{
		"ssh":  {"password": true, "ssh_key": true},
		"http": {"http_bearer": true, "http_basic": true},
	}
	if targetType == "local" {
		if credentialID != nil {
			return validationErr("local target cannot have a credential")
		}
		if value, exists := config["script_root"]; exists {
			if _, ok := value.(string); !ok {
				return validationErr("local target script_root must be a string")
			}
		}
		return nil
	}
	kinds := kindFor[targetType]
	if kinds != nil {
		if credentialID == nil {
			return validationErr(targetType + " target requires a credential")
		}
		credential, err := db.Store.GetCredential(ctx, db.Store.Pool, *credentialID)
		if err != nil {
			return internalError("internal server error")
		}
		if credential == nil || !credential.IsActive {
			return validationErr("credential not found or inactive")
		}
		if !kinds[credential.Kind] {
			return validationErr("credential kind " + credential.Kind + " is invalid for " + targetType)
		}
	}
	if targetType == "ssh" {
		host, _ := config["host"].(string)
		user, _ := config["user"].(string)
		if host == "" || user == "" {
			return validationErr("ssh target requires host and user")
		}
	}
	if targetType == "http" {
		baseURL, _ := config["base_url"].(string)
		if baseURL == "" {
			return validationErr("http target requires base_url")
		}
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
			return validationErr("http base_url must use http or https")
		}
	}
	return nil
}

func targetOut(t *store.ExecutionTarget) TargetOut {
	return TargetOut{
		ID: t.ID, Name: t.Name, Type: t.Type, Config: t.Config.Map(),
		CredentialID: t.CredentialID, IsActive: t.IsActive,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Version: t.Version,
	}
}

func credentialOut(c *store.Credential) CredentialOut {
	return CredentialOut{
		ID: c.ID, Name: c.Name, Kind: c.Kind, Metadata: c.Metadata.Map(),
		IsActive: c.IsActive, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		Version: c.Version,
	}
}

func (s *Server) listTargets(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	p, apiErr := parsePage(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	items, total, err := s.Store.ListTargets(r.Context(), s.Store.Pool, p.Limit, p.Offset, parseOptionalString(r, "type"))
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	p.setTotal(total)
	out := make([]TargetOut, 0, len(items))
	for i := range items {
		out = append(out, targetOut(&items[i]))
	}
	writeJSON(w, 200, TargetList{Items: out, Page: p.Page, PageSize: p.PageSize, Total: p.Total, TotalPages: p.TotalPages})
}

func (s *Server) createTarget(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body TargetCreate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if len(body.Name) < 1 || len(body.Name) > 120 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if body.Type != "local" && body.Type != "ssh" && body.Type != "http" {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if body.CredentialID != nil && !validUUID(*body.CredentialID) {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if apiErr := validateTarget(s, r.Context(), body.Type, body.Config, body.CredentialID); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	target := &store.ExecutionTarget{
		ID: store.NewID(), Name: body.Name, Type: body.Type, Config: rawMap(body.Config),
		CredentialID: body.CredentialID, IsActive: true, CreatedBy: auth.User.ID,
		CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.Store.CreateTarget(ctx, tx, target); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "target.create", "execution_target", target.ID,
			map[string]any{"type": target.Type}, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 201, targetOut(target))
}

func (s *Server) updateTarget(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "target_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body TargetUpdate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	ctx := r.Context()
	var updated *store.ExecutionTarget
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		target, err := s.Store.GetTarget(ctx, tx, id)
		if err != nil {
			return err
		}
		if target == nil {
			return notFound("target not found")
		}
		if target.Version != expected {
			return conflict("target version conflict")
		}
		config := target.Config.Map()
		if body.Config != nil {
			config = body.Config
		}
		credentialID := target.CredentialID
		if body.CredentialIDSet {
			credentialID = body.CredentialID
		}
		if apiErr := validateTarget(s, ctx, target.Type, config, credentialID); apiErr != nil {
			return apiErr
		}
		if body.Name != nil {
			target.Name = *body.Name
		}
		target.Config = rawMap(config)
		target.CredentialID = credentialID
		if body.IsActive != nil {
			target.IsActive = *body.IsActive
			if !target.IsActive {
				if _, err := s.Store.FailQueuedRunsForTarget(ctx, tx, target.ID, ""); err != nil {
					return err
				}
			}
		}
		target.Version++
		if err := s.Store.SaveTarget(ctx, tx, target); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "target.update", "execution_target", target.ID, nil, r)
		updated = target
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, targetOut(updated))
}

func (s *Server) deleteTarget(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "target_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		target, err := s.Store.GetTarget(ctx, tx, id)
		if err != nil {
			return err
		}
		if target == nil {
			return notFound("target not found")
		}
		if target.Version != expected {
			return conflict("target version conflict")
		}
		active, err := s.Store.FindActiveRunForTarget(ctx, tx, "target_id", target.ID)
		if err != nil {
			return err
		}
		if active {
			return validationErr("target has active runs; cancel them first")
		}
		referenced, err := s.Store.FindVersionReferencingTarget(ctx, tx, target.ID)
		if err != nil {
			return err
		}
		if referenced {
			return validationErr("target is referenced by a task definition version")
		}
		s.audit(ctx, tx, auth.User, "target.delete", "execution_target", target.ID,
			map[string]any{"name": target.Name, "type": target.Type}, r)
		return s.Store.DeleteTarget(ctx, tx, target.ID)
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listCredentials(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	p, apiErr := parsePage(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	items, total, err := s.Store.ListCredentials(r.Context(), s.Store.Pool, p.Limit, p.Offset)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	p.setTotal(total)
	out := make([]CredentialOut, 0, len(items))
	for i := range items {
		out = append(out, credentialOut(&items[i]))
	}
	writeJSON(w, 200, CredentialList{Items: out, Page: p.Page, PageSize: p.PageSize, Total: p.Total, TotalPages: p.TotalPages})
}

func validCredentialKind(kind string) bool {
	return kind == "password" || kind == "ssh_key" || kind == "http_bearer" || kind == "http_basic"
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body CredentialCreate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if len(body.Name) < 1 || len(body.Name) > 120 || !validCredentialKind(body.Kind) || len(body.Secret) < 1 || len(body.Secret) > 10000 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	id := store.NewID()
	ciphertext, keyVersion, err := core.EncryptCredential(s.Box, id, body.Kind, body.Secret)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	now := time.Now().UTC()
	credential := &store.Credential{
		ID: id, Name: body.Name, Kind: body.Kind, Ciphertext: ciphertext,
		KeyVersion: keyVersion, Metadata: rawMap(body.Metadata), IsActive: true,
		CreatedBy: auth.User.ID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	ctx := r.Context()
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.Store.CreateCredential(ctx, tx, credential); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "credential.create", "credential", credential.ID,
			map[string]any{"kind": credential.Kind}, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 201, credentialOut(credential))
}

func (s *Server) updateCredential(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "credential_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body CredentialUpdate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	ctx := r.Context()
	var updated *store.Credential
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		credential, err := s.Store.GetCredential(ctx, tx, id)
		if err != nil {
			return err
		}
		if credential == nil {
			return notFound("credential not found")
		}
		if credential.Version != expected {
			return conflict("credential version conflict")
		}
		if body.Name != nil {
			credential.Name = *body.Name
		}
		if body.Metadata != nil {
			credential.Metadata = rawMap(body.Metadata)
		}
		if body.Secret != nil {
			ciphertext, keyVersion, err := core.EncryptCredential(s.Box, credential.ID, credential.Kind, *body.Secret)
			if err != nil {
				return err
			}
			credential.Ciphertext = ciphertext
			credential.KeyVersion = keyVersion
		}
		if body.IsActive != nil {
			credential.IsActive = *body.IsActive
			if !credential.IsActive {
				if _, err := s.Store.FailQueuedRunsForTarget(ctx, tx, "", credential.ID); err != nil {
					return err
				}
			}
		}
		credential.Version++
		if err := s.Store.SaveCredential(ctx, tx, credential); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "credential.update", "credential", credential.ID, nil, r)
		updated = credential
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, credentialOut(updated))
}

func (s *Server) deleteCredential(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "credential_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		credential, err := s.Store.GetCredential(ctx, tx, id)
		if err != nil {
			return err
		}
		if credential == nil {
			return notFound("credential not found")
		}
		if credential.Version != expected {
			return conflict("credential version conflict")
		}
		referenced, err := s.Store.FindTargetUsingCredential(ctx, tx, credential.ID)
		if err != nil {
			return err
		}
		if referenced {
			return validationErr("credential is referenced by an execution target")
		}
		active, err := s.Store.FindActiveRunForTarget(ctx, tx, "credential_id", credential.ID)
		if err != nil {
			return err
		}
		if active {
			return validationErr("credential has active runs; cancel them first")
		}
		s.audit(ctx, tx, auth.User, "credential.delete", "credential", credential.ID,
			map[string]any{"name": credential.Name, "kind": credential.Kind}, r)
		return s.Store.DeleteCredential(ctx, tx, credential.ID)
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
