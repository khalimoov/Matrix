package api

import (
	"net/http"
	"strings"

	"matrix/internal/store"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string     `json:"username"`
		Password string     `json:"password"`
		Role     store.Role `json:"role"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Username) > 64 {
		writeError(w, http.StatusBadRequest, "имя пользователя: от 1 до 64 символов")
		return
	}
	if !req.Role.Valid() {
		writeError(w, http.StatusBadRequest, "роль: admin, operator или viewer")
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.store.CreateUser(req.Username, hash, req.Role)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.monitor.Emit(store.EventUserChanged, 0, "Пользователь "+currentUser(r).Username+" создал пользователя "+u.Username)
	writeJSON(w, http.StatusCreated, u)
}

// updateUser changes a user's role and, if given, password.
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Role     store.Role `json:"role"`
		Password string     `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	u, err := s.store.UserByID(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !req.Role.Valid() {
		writeError(w, http.StatusBadRequest, "роль: admin, operator или viewer")
		return
	}
	if u.Role == store.RoleAdmin && req.Role != store.RoleAdmin && !s.hasOtherAdmin() {
		writeError(w, http.StatusBadRequest, "нельзя понизить последнего администратора")
		return
	}
	if req.Password != "" {
		hash, err := HashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.UpdateUserPassword(id, hash); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	if err := s.store.UpdateUserRole(id, req.Role); err != nil {
		writeStoreError(w, err)
		return
	}
	u.Role = req.Role
	s.monitor.Emit(store.EventUserChanged, 0, "Пользователь "+currentUser(r).Username+" изменил пользователя "+u.Username)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if id == currentUser(r).ID {
		writeError(w, http.StatusBadRequest, "нельзя удалить самого себя")
		return
	}
	u, err := s.store.UserByID(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if u.Role == store.RoleAdmin && !s.hasOtherAdmin() {
		writeError(w, http.StatusBadRequest, "нельзя удалить последнего администратора")
		return
	}
	if err := s.store.DeleteUser(id); err != nil {
		writeStoreError(w, err)
		return
	}
	s.monitor.Emit(store.EventUserChanged, 0, "Пользователь "+currentUser(r).Username+" удалил пользователя "+u.Username)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) hasOtherAdmin() bool {
	n, err := s.store.CountAdmins()
	return err == nil && n > 1
}
